/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodereaper

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	controlPlaneType = "control-plane"
)

type NodeSelector string

const (
	nodeSelectorAll          NodeSelector = ""
	nodeSelectorNode         NodeSelector = "node-role.kubernetes.io/node="
	nodeSelectorControlPlane NodeSelector = "node-role.kubernetes.io/control-plane="
	controlPlaneNodeLabel    string       = "node-role.kubernetes.io/control-plane"
	lockTableClusterIDKey                 = "ClusterID"
)

func parseTaint(t string) (v1.Taint, bool, error) {
	var key, value string
	var effect v1.TaintEffect
	var taint v1.Taint

	parts := strings.Split(t, ":")

	switch len(parts) {
	case 1:
		key = parts[0]
	case 2:
		effect = v1.TaintEffect(parts[1])
		KV := strings.Split(parts[0], "=")

		if len(KV) > 2 {
			return taint, false, errors.Errorf("invalid taint %v provided", t)
		}

		key = KV[0]

		if len(KV) == 2 {
			value = KV[1]
		}
	default:
		return taint, false, errors.Errorf("invalid taint %v provided", t)
	}

	taint.Key = key
	taint.Value = value
	taint.Effect = effect
	taint.TimeAdded = &metav1.Time{Time: time.Time{}}
	return taint, true, nil
}

func runCommand(call string, arg []string) (string, error) {
	log.Infof("invoking >> %s %s", call, arg)
	out, err := exec.Command(call, arg...).CombinedOutput()
	if err != nil {
		log.Errorf("call failed with output: %s,  error: %s", string(out), err)
		return string(out), err
	}
	log.Infof("call succeeded with output: %s", string(out))
	return string(out), err
}

func runCommandWithContext(call string, args []string, timeoutSeconds int64) (string, error) {
	// Create a new context and add a timeout to it
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, call, args...)
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		timeoutErr := fmt.Errorf("command execution timed out")
		log.Error(timeoutErr)
		return string(out), timeoutErr
	}

	if err != nil {
		log.Errorf("call failed with output: %s,  error: %s", string(out), err)
		return string(out), err
	}
	return string(out), nil
}

func (ctx *ReaperContext) uncordonNode(name string, dryRun bool, ignoreDrainFailure bool) error {
	uncordonArgs := []string{"uncordon", name}
	uncordonCommand := ctx.KubectlLocalPath
	if dryRun || ignoreDrainFailure {
		log.Warnf("dry run / ignore drain failure is on, instance %v remains cordoned", name)
	} else {
		_, err := runCommand(uncordonCommand, uncordonArgs)
		if err != nil {
			log.Errorf("failed to uncordon node %v", name)
			return err
		}
	}
	return nil
}

func (ctx *ReaperContext) terminateInstance(w autoscalingiface.AutoScalingAPI, id string, nodeName string) error {

	terminateInput := &autoscaling.TerminateInstanceInAutoScalingGroupInput{
		InstanceId:                     &id,
		ShouldDecrementDesiredCapacity: aws.Bool(false),
	}

	_, err := w.TerminateInstanceInAutoScalingGroup(terminateInput)
	if err != nil {
		return err
	}

	if err := ctx.annotateNode(nodeName, stateAnnotationKey, terminatedStateName); err != nil {
		log.Warnf("failed to update state annotation on node '%v'", nodeName)
	}

	log.Info("instance terminate event occurred")
	return nil
}

func (ctx *ReaperContext) drainNode(name string, dryRun bool) error {
	log.Infof("draining node %v", name)
	drainArgs := []string{"drain", name, "--ignore-daemonsets=true", "--delete-local-data=true", "--force", "--grace-period=-1"}
	drainCommand := ctx.KubectlLocalPath
	if dryRun {
		log.Warnf("dry run is on, instance not drained")
	} else {
		if err := ctx.annotateNode(name, stateAnnotationKey, drainingStateName); err != nil {
			log.Warnf("failed to update state annotation on node '%v'", name)
		}
		cmdOut, err := runCommandWithContext(drainCommand, drainArgs, ctx.DrainTimeoutSeconds)
		log.Infof("drain command output: %s", cmdOut)
		if err != nil {
			event := ctx.getUnreapableDrainFailureEvent(name, err.Error())
			ctx.publishEvent(ctx.SelfNamespace, event)
			if err.Error() == "command execution timed out" {
				log.Warnf("failed to drain node %v, drain command timed-out", name)
				ctx.annotateNode(name, ageUnreapableAnnotationKey, getUTCNowStr())
				ctx.uncordonNode(name, dryRun, ctx.IgnoreFailure)
				return err
			}
			log.Warnf("failed to drain node: %v", err)
			ctx.uncordonNode(name, dryRun, ctx.IgnoreFailure)
			return err
		}
		ctx.DrainedInstances++
	}
	return nil
}

func (ctx *ReaperContext) getUnreapableDrainFailureEvent(nodeName, message string) *v1.Event {
	event := &v1.Event{
		Reason:  "NodeDrainFailed",
		Message: fmt.Sprintf("Node %v is unreapable: %v", nodeName, message),
		Type:    "Warning",
		LastTimestamp: metav1.Time{
			Time: time.Now(),
		},
	}
	event.SetName(fmt.Sprintf("node-reaper.%v", strconv.FormatInt(time.Now().UTC().UnixNano(), 10)))
	event.SetNamespace(ctx.SelfNamespace)
	event.InvolvedObject.Kind = "Node"
	event.InvolvedObject.Name = ctx.SelfName
	event.InvolvedObject.Namespace = ctx.SelfNamespace
	return event
}

func (ctx *ReaperContext) annotateNode(nodeName, annotationKey, annotationValue string) error {
	annotation := fmt.Sprintf("%v=%v", annotationKey, annotationValue)
	annotateArgs := []string{"annotate", "--overwrite", "node", nodeName, annotation}
	annotateCommand := ctx.KubectlLocalPath
	if ctx.DryRun {
		log.Warnf("dry run is on, node not annotated")
	} else {
		_, err := runCommand(annotateCommand, annotateArgs)
		if err != nil {
			log.Errorf("failed to annotate node %v", nodeName)
			return err
		}
	}
	return nil
}

func (ctx *ReaperContext) publishEvent(namespace string, event *v1.Event) error {
	log.Infof("publishing event: %v", event.Reason)
	_, err := ctx.KubernetesClient.CoreV1().Events(namespace).Create(event)
	if err != nil {
		log.Errorf("failed to publish event: %v", err)
		return err
	}
	return nil
}

func (ctx *ReaperContext) obtainReapLock(ddbAPI dynamodbiface.DynamoDBAPI, nodeName, instanceID, nodeType string) (LockRecord, error) {
	log.Infof("obtaining lock for a %s node %s (%s)", nodeType, nodeName, instanceID)

	timestamp := time.Now().Format(time.RFC3339)

	lock := LockRecord{
		LockType:   nodeType,
		ClusterID:  ctx.ClusterID,
		NodeName:   nodeName,
		InstanceID: instanceID,
		CreatedAt:  timestamp,
		tableName:  ctx.LocksTableName,
	}

	err := lock.obtainLock(ddbAPI)
	return lock, err
}

func (l LockRecord) obtainLock(ddbAPI dynamodbiface.DynamoDBAPI) error {
	serializedLock, err := dynamodbattribute.MarshalMap(l)
	if err != nil {
		return err
	}
	input := &dynamodb.PutItemInput{
		Item:                serializedLock,
		TableName:           aws.String(l.tableName),
		ConditionExpression: aws.String("attribute_not_exists(LockType)"),
	}

	_, err = ddbAPI.PutItem(input)
	if err != nil {
		log.Infof("failed to obtain lock for a %s node %s (%s): %s", controlPlaneType, l.NodeName, l.InstanceID, err.Error())
		return err
	}

	l.locked = true

	log.Infof("successfully obtained lock for a %s node %s (%s)", controlPlaneType, l.NodeName, l.InstanceID)

	return err
}

// TODO: should this be (ddbAPI, lock) instead? Or Lock.tryClearLock(ddbAPI)?
func (ctx *ReaperContext) tryClearLock(ddbAPI dynamodbiface.DynamoDBAPI, err error, nodeName, instanceID string) {
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
			// check if we need to do lock cleanup here

			result, err := ddbAPI.GetItem(&dynamodb.GetItemInput{
				ProjectionExpression: aws.String(fmt.Sprintf("LockType,InstanceID,%s", lockTableClusterIDKey)),
				Key: map[string]*dynamodb.AttributeValue{
					"LockType": {
						S: aws.String(controlPlaneType),
					},
				},
				TableName: aws.String(ctx.LocksTableName),
			})

			if err != nil || result.Item == nil {
				// don't care, skip this node then
				log.Errorf("failed to get lock record for cluster %s: %s", ctx.ClusterID, err)
				return
			}

			item := LockRecord{
				tableName: ctx.LocksTableName,
			}

			err = dynamodbattribute.UnmarshalMap(result.Item, &item)
			if err != nil {
				log.Errorf("failed to unmarshal lock record for cluster %s: %s", ctx.ClusterID, err)
				return
			}

			// this lock belongs to this cluster and should have been released
			if item.ClusterID == ctx.ClusterID {
				if err := item.releaseLock(ddbAPI); err != nil {
					log.Errorf("failed to clean up a leftover lock for cluster %s: %s", ctx.ClusterID, err)
				}
			} else {
				log.Infof("another master roll is in progress, skipping node %s (%s)", nodeName, instanceID)
			}
		} else {
			log.Infof("AWS error while attempting to obtain lock for %s (%s), skipping: %s", nodeName, instanceID, aerr.Message())
		}
	} else {
		log.Infof("unknown error while attempting to obtain lock for %s (%s), skipping: %s", nodeName, instanceID, err.Error())
	}
}

func (l LockRecord) releaseLock(ddbAPI dynamodbiface.DynamoDBAPI) error {
	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"LockType": {
				S: aws.String(controlPlaneType),
			},
		},
		TableName:           aws.String(l.tableName),
		ConditionExpression: aws.String("ClusterID = :cid"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":cid": {
				S: aws.String(l.ClusterID),
			},
		},
	}

	_, err := ddbAPI.DeleteItem(input)
	if err != nil {
		log.Infof("failed to release lock for a %s node %s (%s): %s", controlPlaneType, l.NodeName, l.InstanceID, err.Error())
		return err
	}

	l.locked = false

	log.Infof("successfully released lock for a %s node %s (%s)", controlPlaneType, l.NodeName, l.InstanceID)
	return nil
}

func nodeHasActivePods(n *v1.Node, allPods []v1.Pod) bool {
	nodeName := n.ObjectMeta.Name
	log.Infof("inspecting pods assigned to %v", nodeName)
	for _, pod := range allPods {
		if pod.Spec.NodeName == nodeName {
			if pod.Status.Reason == "NodeLost" {
				continue
			}
			podConditions := pod.Status.Conditions
			for _, condition := range podConditions {
				if condition.Type == "Ready" {
					if condition.Status == "True" {
						return true
					}
				}
			}
		}
	}
	return false
}

func getNodeInstanceID(n *v1.Node) string {
	providerID := n.Spec.ProviderID
	splitProviderID := strings.Split(providerID, "/")
	instanceID := splitProviderID[len(splitProviderID)-1]
	return instanceID
}

func getNodeAgeMinutes(n *v1.Node) int {
	now := time.Now().UTC()
	createdDate := n.ObjectMeta.GetCreationTimestamp().UTC()
	nodeAge := int(now.Sub(createdDate).Minutes())
	return nodeAge
}

func getNodeRegion(n *v1.Node) string {
	var regionName = ""
	labels := n.GetLabels()
	if labels != nil {
		regionName = labels["topology.kubernetes.io/region"]
	}
	if regionName == "" {
		providerID := n.Spec.ProviderID
		splitProviderID := strings.Split(providerID, "/")
		regionFullName := splitProviderID[len(splitProviderID)-2]
		regionName = regionFullName[:len(regionFullName)-1]
	}
	return regionName
}

func getLastTransitionDurationMinutes(n *v1.Node) float64 {
	var minuteDiff float64
	now := time.Now().UTC()
	conditions := n.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			transitionTimestamp := condition.LastTransitionTime.UTC()
			minuteDiff = now.Sub(transitionTimestamp).Minutes()
		}
	}
	return minuteDiff
}

func nodeStateIsNotReady(n *v1.Node) bool {
	conditions := n.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			if condition.Status == "False" {
				return true
			}
		}
	}
	return false
}

func nodeStateIsReady(n *v1.Node) bool {
	conditions := n.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			if condition.Status == "True" {
				return true
			}
		}
	}
	return false
}

func nodeStateIsUnknown(n *v1.Node) bool {
	conditions := n.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			if condition.Status == "Unknown" {
				return true
			}
		}
	}
	return false
}

func getInstanceTagValue(w ec2iface.EC2API, instance string, key string) (string, error) {
	filters := []*ec2.Filter{
		{Name: aws.String("resource-id"), Values: []*string{&instance}},
		{Name: aws.String("key"), Values: []*string{&key}}}
	describeTagsInput := &ec2.DescribeTagsInput{Filters: filters}
	response, err := w.DescribeTags(describeTagsInput)
	if err != nil {
		return "", err
	}
	if len(response.Tags) != 1 {
		err := fmt.Errorf("failed to find tag %v for instance %v", key, instance)
		return "", err
	}
	return *response.Tags[0].Value, nil
}

func getAutoScalingGroup(w autoscalingiface.AutoScalingAPI, name string) (autoscaling.Group, error) {
	describeInput := &autoscaling.DescribeAutoScalingGroupsInput{AutoScalingGroupNames: []*string{&name}}
	response, err := w.DescribeAutoScalingGroups(describeInput)
	if err != nil {
		return autoscaling.Group{}, err
	}
	if len(response.AutoScalingGroups) != 1 {
		err := fmt.Errorf("failed to find ASG %v", name)
		return autoscaling.Group{}, err
	}
	return *response.AutoScalingGroups[0], nil
}

func dumpSpec(nodeName string, kubeClient kubernetes.Interface) error {
	nodeObject, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	nodeDump, err := json.Marshal(nodeObject)
	if err != nil {
		return err
	}
	log.Infof("node dump: %v", string(nodeDump))
	return nil
}

func nodeMeetsReapAfterThreshold(minuteThreshold float64, minutesSinceTransition float64) bool {
	if minutesSinceTransition > minuteThreshold {
		return true
	}
	return false
}

func isControlPlane(node string, kubeClient kubernetes.Interface) (bool, error) {
	corev1 := kubeClient.CoreV1()
	nodeObject, err := corev1.Nodes().Get(node, metav1.GetOptions{})
	if err != nil {
		log.Errorf("failed to get node, %v", err)
		return false, err
	}
	labels := nodeObject.ObjectMeta.GetLabels()
	if labels[controlPlaneNodeLabel] == "" {
		return true, nil
	}
	return false, nil
}

func getHealthyMasterCount(kubeClient kubernetes.Interface) (int, error) {
	corev1 := kubeClient.CoreV1()
	masterCount := 0

	nodeList, err := corev1.Nodes().List(metav1.ListOptions{LabelSelector: string(nodeSelectorControlPlane)})
	if err != nil {
		log.Errorf("failed to list master nodes, %v", err)
		return 0, err
	}
	for _, node := range nodeList.Items {
		if nodeStateIsReady(&node) {
			masterCount++
		}
	}
	return masterCount, nil
}

func (ctx *ReaperContext) waitForNodesReady(selector NodeSelector) error {
	var controlPlaneCheckError error
	// Do not release the lock until control plane is healthy
	controlPlaneHealthCheckStart := time.Now()
	maxWait := time.Second * time.Duration(ctx.NodeHealthcheckTimeoutSeconds)

	for time.Since(controlPlaneHealthCheckStart) < maxWait {
		controlPlaneReady, err := allNodesAreReady(ctx.KubernetesClient, selector)
		if controlPlaneReady {
			controlPlaneCheckError = nil
			break
		}
		if err != nil {
			log.Infof("waiting for control plane to become healthy before releasing the lock")
			controlPlaneCheckError = err
		}

		if !controlPlaneReady {
			log.Infof("waiting for control plane to become healthy before releasing the lock")
		}

		time.Sleep(time.Second * time.Duration(ctx.NodeHealthcheckIntervalSeconds))
	}

	return controlPlaneCheckError
}

func allNodesAreReady(kubeClient kubernetes.Interface, nodeType NodeSelector) (bool, error) {
	corev1 := kubeClient.CoreV1()

	opts := metav1.ListOptions{}

	if nodeType != nodeSelectorAll {
		opts.LabelSelector = string(nodeType)
	}

	nodeList, err := corev1.Nodes().List(opts)
	if err != nil {
		log.Errorf("failed to list all nodes, %v", err)
		return false, err
	}

	for _, node := range nodeList.Items {
		if nodeStateIsNotReady(&node) || nodeStateIsUnknown(&node) {
			return false, nil
		}
	}
	return true, nil
}

func isTerminated(instances []*ec2.Instance, instanceID string) bool {
	var terminatedStateName = "terminated"
	for _, instance := range instances {
		if aws.StringValue(instance.InstanceId) == instanceID {
			if aws.StringValue(instance.State.Name) == terminatedStateName {
				return true
			}
		}
	}
	return false
}

func getInstanceIDByPrivateDNS(instances []*ec2.Instance, dnsName string) string {
	var runningStateName = "running"
	for _, instance := range instances {
		if aws.StringValue(instance.PrivateDnsName) == dnsName {
			if aws.StringValue(instance.State.Name) == runningStateName {
				return aws.StringValue(instance.InstanceId)
			}
		}
	}
	return ""
}

func getUTCNowStr() string {
	return time.Now().UTC().Format(time.RFC3339)
}
