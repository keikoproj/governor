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
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var log = logrus.New()

const (
	ageUnreapableAnnotationKey  = "governor.keikoproj.io/age-unreapable"
	stateAnnotationKey          = "governor.keikoproj.io/state"
	terminatedStateName         = "termination-issued"
	drainingStateName           = "draining"
	reaperDisableLabelKey       = "governor.keikoproj.io/node-reaper-disabled"
	reapUnreadyDisabledLabelKey = "governor.keikoproj.io/reap-unready-disabled"
	reapUnknownDisabledLabelKey = "governor.keikoproj.io/reap-unknown-disabled"
	reapFlappyDisabledLabelKey  = "governor.keikoproj.io/reap-flappy-disabled"
	reapOldDisabledLabelKey     = "governor.keikoproj.io/reap-old-disabled"

	NodeReaperResultMetricName = "governor_node_reaper_result"
	drainFailedMetric          = "DrainFailedAgeExpiredNode"
	terminationReasonUnhealthy = "TerminateUnhealthyNode"
	terminationReasonHealthy   = "TerminateAgeExpiredNode"
)

// Validate command line arguments
func (ctx *ReaperContext) validateArguments(args *Args) error {
	ctx.DryRun = args.DryRun
	ctx.ReapUnready = args.ReapUnready
	ctx.ReapUnknown = args.ReapUnknown
	ctx.ReapFlappy = args.ReapFlappy
	ctx.ReapGhost = args.ReapGhost
	ctx.ReapUnjoined = args.ReapUnjoined
	ctx.ReapThrottle = args.ReapThrottle
	ctx.AgeReapThrottle = args.AgeReapThrottle
	ctx.SoftReap = args.SoftReap
	ctx.AsgValidation = args.AsgValidation
	ctx.ReapableInstances = make([]ReapableInstance, 0)
	ctx.DrainableInstances = make(map[string]string)
	ctx.ClusterInstancesData = make(map[string]float64)
	ctx.GhostInstances = make(map[string]string)
	ctx.NodeInstanceIDs = make(map[string]string)
	ctx.AgeDrainReapableInstances = make([]AgeDrainReapableInstance, 0)
	ctx.AgeKillOrder = make([]string, 0)
	ctx.ReapTainted = make([]v1.Taint, 0)
	ctx.EC2Region = args.EC2Region
	ctx.ReapOld = args.ReapOld
	ctx.MaxKill = args.MaxKill
	ctx.ControlPlaneNodeCount = args.ControlPlaneNodeCount

	log.Infof("AWS Region = %v", ctx.EC2Region)
	log.Infof("Dry Run = %t", ctx.DryRun)
	log.Infof("Soft Reap = %t", ctx.SoftReap)
	log.Infof("Max Kills = %v", ctx.MaxKill)
	log.Infof("ASG Validation = %t", ctx.AsgValidation)
	log.Infof("Post Reap Throttle = %v seconds", ctx.ReapThrottle)

	reapTaintedLog := []string{}
	for _, t := range args.ReapTainted {
		var taint v1.Taint
		var ok bool
		var err error

		if taint, ok, err = parseTaint(t); !ok {
			return errors.Wrap(err, "failed to parse taint")
		}

		ctx.ReapTainted = append(ctx.ReapTainted, taint)
		reapTaintedLog = append(reapTaintedLog, taint.ToString())
	}

	if len(ctx.ReapTainted) > 0 {
		log.Infof("Reap Tainted = %s", strings.Join(reapTaintedLog, ","))
	}

	if ctx.MaxKill < 1 {
		err := fmt.Errorf("--max-kill-nodes must be set to a number greater than or equal to 1")
		log.Errorln(err)
		return err
	}

	if ctx.ReapFlappy {
		if args.FlapCount < 1 {
			err := fmt.Errorf("--flap-count must be set to a number greater than or equal to 1")
			log.Errorln(err)
			return err
		}
		ctx.FlapCount = args.FlapCount
		log.Infof("Reap Flappy = %t, threshold = %v events", ctx.ReapFlappy, ctx.FlapCount)
	}

	if ctx.ReapOld {
		if args.ReapOldThresholdMinutes < 1 {
			err := fmt.Errorf("--reap-old-threshold-minutes must be set to a number greater than or equal to 1")
			log.Errorln(err)
			return err
		}
		ctx.ReapOldThresholdMinutes = args.ReapOldThresholdMinutes
		log.Infof("Reap Old = %t, threshold = %v minutes", ctx.ReapOld, ctx.ReapOldThresholdMinutes)

		if ctx.ReapOldThresholdMinutes < 10080 {
			log.Warnf("--reap-old-threshold-minutes is set to %v - reaping nodes younger than 7 days is not recommended", ctx.ReapOldThresholdMinutes)
		}
	}

	if !ctx.ReapUnready && !ctx.ReapUnknown && !ctx.ReapFlappy && !ctx.ReapOld {
		log.Warnf("all reap flags are off !! nodes will never be reaped")
	}

	if args.ReapAfter < 1 {
		err := fmt.Errorf("--reap-after must be set to a number greater than or equal to 1")
		log.Errorln(err)
		return err
	}
	ctx.TimeToReap = args.ReapAfter

	if args.ReconsiderUnreapableAfter < 10 {
		err := fmt.Errorf("--reconsider-unreapable-after must be set to a number greater than or equal to 10")
		log.Errorln(err)
		return err
	}

	ctx.ReconsiderUnreapableAfter = args.ReconsiderUnreapableAfter

	if args.ReapUnjoined {
		if args.ReapUnjoinedThresholdMinutes < 10 {
			err := fmt.Errorf("--reap-unjoined-threshold-minutes must be set to a number greater than or equal to 10")
			log.Errorln(err)
			return err
		}
		ctx.ReapUnjoinedThresholdMinutes = args.ReapUnjoinedThresholdMinutes

		if args.ReapUnjoinedKey == "" {
			err := fmt.Errorf("--reap-unjoined-tag-key must be set to an ec2 tag key")
			log.Errorln(err)
			return err
		}
		ctx.ReapUnjoinedKey = args.ReapUnjoinedKey

		if args.ReapUnjoinedValue == "" {
			err := fmt.Errorf("--reap-unjoined-tag-value must be set to an ec2 tag value")
			log.Errorln(err)
			return err
		}
		ctx.ReapUnjoinedValue = args.ReapUnjoinedValue
	}

	if args.DrainTimeoutSeconds < 600 {
		err := fmt.Errorf("--drain-timeout must be set to number greater than or equal to 600")
		log.Errorln(err)
		return err
	}

	ctx.DrainTimeoutSeconds = args.DrainTimeoutSeconds

	ctx.NodeHealthcheckTimeoutSeconds = args.NodeHealthcheckTimeoutSeconds

	log.Infof("Reap Unknown = %t, threshold = %v minutes", ctx.ReapUnknown, ctx.TimeToReap)
	log.Infof("Reap Unready = %t, threshold = %v minutes", ctx.ReapUnready, ctx.TimeToReap)
	log.Infof("Reap Ghost = %t, threshold = immediate", ctx.ReapGhost)
	log.Infof("Reap Unjoined = %t, threshold = %v minutes by tag %v=%v", ctx.ReapUnjoined, ctx.ReapUnjoinedThresholdMinutes, ctx.ReapUnjoinedKey, ctx.ReapUnjoinedValue)
	log.Infof("Reconsider Unreapable after = %v minutes", ctx.ReconsiderUnreapableAfter)
	log.Infof("Drain Timeout = %d seconds", ctx.DrainTimeoutSeconds)
	log.Infof("Node Healthcheck Timeout = %d seconds", ctx.NodeHealthcheckTimeoutSeconds)

	if !ctx.SoftReap {
		log.Warnf("--soft-reap is off !! will not consider pods when reaping")
	}

	if args.KubectlLocalPath != "" {
		ok := common.PathExists(args.KubectlLocalPath)
		if !ok {
			err := fmt.Errorf("--kubectl path '%v' does not exist", args.KubectlLocalPath)
			log.Errorln(err)
			return err
		}
		ctx.KubectlLocalPath = args.KubectlLocalPath
	}

	if ctx.ReapFlappy && ctx.KubectlLocalPath == "" {
		err := fmt.Errorf("must provide --kubectl path if --reap-flappy is true")
		return err
	}

	if args.K8sConfigPath != "" {
		ok := common.PathExists(args.K8sConfigPath)
		if !ok {
			err := fmt.Errorf("--kubeconfig path '%v' does not exist", ctx.KubernetesConfigPath)
			log.Errorln(err)
			return err
		}
		ctx.KubernetesConfigPath = args.K8sConfigPath
	}

	if args.LocalMode {
		var err error
		ctx.KubernetesClient, err = common.OutOfClusterAuth(ctx.KubernetesConfigPath)
		if err != nil {
			log.Errorln("cluster external auth failed")
			return err
		}
		ctx.SelfNode = "localmode"
		ctx.SelfName = "node-reaper"
		ctx.SelfNamespace = "default"
	} else {
		var err error
		ctx.KubernetesClient, err = common.InClusterAuth()
		if err != nil {
			log.Errorln("in-cluster auth failed")
			return err
		}
	}

	if args.LocksTableName != "" {
		if args.ClusterID == "" {
			err := fmt.Errorf("must provide --cluster-id if --locks-table-name is set")
			return err
		}

		ctx.ClusterID = args.ClusterID
		ctx.LocksTableName = args.LocksTableName

		log.Infof("Cluster ID = %s", ctx.ClusterID)
		log.Infof("Locks Table Name = %s", ctx.LocksTableName)
	}

	return nil
}

// Run is the main runner function for node-reaper, and will initialize and start the node-reaper
func Run(args *Args) error {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	ctx := &ReaperContext{}

	if args.PromPushgateway != "" {
		ctx.MetricsAPI = common.NewPrometheusAPI(args.PromPushgateway)
	}

	err := ctx.validateArguments(args)
	if err != nil {
		log.Errorf("failed to parse commandline arguments, %v", err)
		return err
	}

	var config aws.Config
	var awsAuth ReaperAwsAuth

	config.Region = &ctx.EC2Region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            config,
	}))

	awsAuth.EC2 = ec2.New(sess)
	awsAuth.ASG = autoscaling.New(sess)
	awsAuth.DDB = dynamodb.New(sess)

	log.Infoln("starting api scanner")
	err = ctx.scan(awsAuth)
	if err != nil {
		log.Errorf("failed to scan nodes, %v", err)
		return err
	}

	log.Infoln("starting drain condition check for flappy nodes")
	err = ctx.deriveFlappyDrainReapableNodes()
	if err != nil {
		log.Errorf("failed to derive flappy drain-reapable nodes, %v", err)
		return err
	}

	log.Infoln("starting drain condition check for old nodes")
	err = ctx.deriveAgeDrainReapableNodes()
	if err != nil {
		log.Errorf("failed to derive age drain-reapable nodes, %v", err)
		return err
	}

	log.Infoln("starting drain condition check for ghost nodes")
	err = ctx.deriveGhostDrainReapableNodes(awsAuth)
	if err != nil {
		log.Errorf("failed to derive ghost nodes, %v", err)
		return err
	}

	log.Infoln("starting drain condition check for tainted nodes")
	err = ctx.deriveTaintDrainReapableNodes()
	if err != nil {
		log.Errorf("failed to derive taint drain-reapable nodes, %v", err)
		return err
	}

	log.Infoln("starting reap condition check")
	err = ctx.deriveReapableNodes()
	if err != nil {
		log.Errorf("failed to derive reapable nodes, %v", err)
		return err
	}

	if len(ctx.ReapableInstances) != 0 {
		log.Infoln("starting reap cycle for unhealthy nodes")
		err = ctx.reapUnhealthyNodes(awsAuth)
		if err != nil {
			log.Errorf("failed to reap unhealthy nodes, %v", err)
			return err
		}
	} else {
		log.Infoln("no unhealthy reapable nodes found")
	}

	if len(ctx.AgeDrainReapableInstances) != 0 {
		log.Infoln("starting reap cycle for old nodes")
		err = ctx.reapOldNodes(awsAuth)
		if err != nil {
			log.Errorf("failed to reap old nodes, %v", err)
			return err
		}
	} else {
		log.Infoln("no old reapable nodes found")
	}

	return nil
}

func (ctx *ReaperContext) deriveTaintDrainReapableNodes() error {
	if len(ctx.ReapTainted) == 0 {
		return nil
	}

	log.Infoln("scanning for taint drain-reapable nodes")
	for _, node := range ctx.AllNodes {
		nodeInstanceID := getNodeInstanceID(&node)
		for _, t := range ctx.ReapTainted {
			if nodeIsTainted(t, node) {
				ctx.addDrainable(node.Name, nodeInstanceID)
				ctx.addReapable(node.Name, nodeInstanceID, ctx.AsgValidation)
			}
		}
	}
	return nil
}

// Handle age-reapable nodes
func (ctx *ReaperContext) deriveAgeDrainReapableNodes() error {
	log.Infoln("scanning for age drain-reapable nodes")
	for _, node := range ctx.AllNodes {
		nodeName := node.ObjectMeta.Name
		nodeInstanceID := getNodeInstanceID(&node)
		nodeRegion := getNodeRegion(&node)
		ageThreshold := int(ctx.ReapOldThresholdMinutes)
		nodeAgeMinutes := getNodeAgeMinutes(&node)

		if nodeRegion != ctx.EC2Region {
			log.Infof("node %v is not reapable, running in different region %v", node.ObjectMeta.Name, nodeRegion)
			continue
		}

		// Drain-Reap old nodes
		if ctx.ReapOld {
			if reconsiderUnreapableNode(node, ctx.ReconsiderUnreapableAfter) && !hasSkipLabel(node, reapOldDisabledLabelKey) {
				if nodeIsAgeReapable(nodeAgeMinutes, ageThreshold) {
					log.Infof("node %v is drain-reapable !! State = OldAge, Diff = %v/%v", nodeName, nodeAgeMinutes, ageThreshold)
					ctx.addAgeDrainReapable(nodeName, nodeInstanceID, nodeAgeMinutes)
				}
			}
		}
	}
	return nil
}

// Handle flappy-reapable nodes
func (ctx *ReaperContext) deriveFlappyDrainReapableNodes() error {
	log.Infoln("scanning for flappy drain-reapable nodes")
	for _, node := range ctx.AllNodes {
		nodeName := node.ObjectMeta.Name
		nodeInstanceID := getNodeInstanceID(&node)
		nodeRegion := getNodeRegion(&node)
		countThreshold := ctx.FlapCount
		events := ctx.AllEvents

		if nodeRegion != ctx.EC2Region {
			log.Infof("node %v is not reapable, running in different region %v", node.ObjectMeta.Name, nodeRegion)
			continue
		}

		// Drain-Reap flappy nodes
		if ctx.ReapFlappy {
			if nodeIsFlappy(events, nodeName, countThreshold, "NodeReady") && !hasSkipLabel(node, reapFlappyDisabledLabelKey) {
				log.Infof("node %v is drain-reapable !! State = ReadinessFlapping", nodeName)
				ctx.addDrainable(nodeName, nodeInstanceID)
				ctx.addReapable(nodeName, nodeInstanceID, ctx.AsgValidation)
			}
		}
	}
	return nil
}

// Handle ghost-reapable nodes
func (ctx *ReaperContext) deriveGhostDrainReapableNodes(w ReaperAwsAuth) error {
	log.Infoln("scanning for ghost drain-reapable nodes")
	for instance, node := range ctx.NodeInstanceIDs {
		// skip iteration if instance ID is not a terminated instance
		if !isTerminated(ctx.AllInstances, instance) {
			continue
		}
		// find the real instance id by node name
		realInstanceID := getInstanceIDByPrivateDNS(ctx.AllInstances, node)

		// skip iteration if no running instance with nodeName was found
		if realInstanceID == "" {
			continue
		}
		log.Infof("node %v is referencing terminated instance %v, actual instance is %v", node, instance, realInstanceID)
		ctx.GhostInstances[node] = realInstanceID
	}

	if ctx.ReapGhost {
		for node, instance := range ctx.GhostInstances {
			log.Infof("node %v is drain-reapable, referencing terminated instance %v !! State = Ghost", node, instance)
			ctx.addDrainable(node, instance)
			ctx.addReapable(node, instance, ctx.AsgValidation)
		}
	}
	return nil
}

// Handle Unknown/NotReady reapable nodes
func (ctx *ReaperContext) deriveReapableNodes() error {

	log.Infoln("scanning for unjoined nodes")
	for instanceID, minutesElapsed := range ctx.ClusterInstancesData {
		// if a cluster instance exist which does not map to an existing node
		if _, ok := ctx.NodeInstanceIDs[instanceID]; !ok {
			if minutesElapsed > float64(ctx.ReapUnjoinedThresholdMinutes) {
				log.Infof("instance '%v' has been running for %f minutes but is not joined to cluster", instanceID, minutesElapsed)
				unjoinedNodeName := fmt.Sprintf("unjoined-%v", instanceID)
				ctx.addReapable(unjoinedNodeName, instanceID, false)
			}
		}
	}

	log.Infoln("scanning for dead nodes")
	for _, node := range ctx.UnreadyNodes {
		nodeInstanceID := getNodeInstanceID(&node)
		nodeRegion := getNodeRegion(&node)
		nodeName := node.ObjectMeta.Name
		lastStateDurationIntervalMinutes := getLastTransitionDurationMinutes(&node)

		if nodeRegion != ctx.EC2Region {
			log.Infof("node %v is not reapable, running in different region %v", node.ObjectMeta.Name, nodeRegion)
			continue
		}

		if ctx.SoftReap && nodeHasActivePods(&node, ctx.AllPods) {
			log.Infof("node %v is not reapable, running pods detected", nodeName)
			continue
		}

		if ctx.ReapUnready && nodeStateIsNotReady(&node) && !hasSkipLabel(node, reapUnreadyDisabledLabelKey) {
			if nodeMeetsReapAfterThreshold(ctx.TimeToReap, lastStateDurationIntervalMinutes) {
				log.Infof("node %v is reapable !! State = NotReady, diff: %.2f/%v", nodeName, lastStateDurationIntervalMinutes, ctx.TimeToReap)
				ctx.addReapable(nodeName, nodeInstanceID, ctx.AsgValidation)
			} else {
				log.Infof("node %v is not reapable, time threshold not met", nodeName)
				continue
			}
		}

		if ctx.ReapUnknown && nodeStateIsUnknown(&node) && !hasSkipLabel(node, reapUnknownDisabledLabelKey) {
			if nodeMeetsReapAfterThreshold(ctx.TimeToReap, lastStateDurationIntervalMinutes) {
				log.Infof("node %v is reapable !! State = Unknown, diff: %.2f/%v", nodeName, lastStateDurationIntervalMinutes, ctx.TimeToReap)
				ctx.addReapable(nodeName, nodeInstanceID, ctx.AsgValidation)
			} else {
				log.Infof("node %v is not reapable, time threshold not met", nodeName)
				continue
			}
		}
	}
	return nil
}

func (ctx *ReaperContext) reapOldNodes(w ReaperAwsAuth) error {
	for _, instance := range ctx.AgeDrainReapableInstances {
		ctx.AgeKillOrder = append(ctx.AgeKillOrder, instance.NodeName)
	}
	log.Infof("Kill order: %v", ctx.AgeKillOrder)

	for _, instance := range ctx.AgeDrainReapableInstances {

		if ctx.TerminatedInstances >= ctx.MaxKill {
			log.Infof("max kill nodes reached, %v/%v nodes have been terminated in current run", ctx.TerminatedInstances, ctx.MaxKill)
			return nil
		}

		// Skip if target node is self
		if instance.NodeName == ctx.SelfNode {
			log.Infof("self node termination attempted, skipping")
			continue
		}

		masterCount, err := getHealthyMasterCount(ctx.KubernetesClient)
		if err != nil {
			return err
		}

		isControlPlaneNode, err := isControlPlane(instance.NodeName, ctx.KubernetesClient)
		if err != nil {
			return err
		}
		// Must have 3 healthy masters in order to terminate a master node
		if isControlPlaneNode {
			if masterCount < ctx.ControlPlaneNodeCount {
				log.Infof("%v", masterCount)
				log.Infof("less than %d healthy master nodes, skipping %v", ctx.ControlPlaneNodeCount, instance.NodeName)
				continue
			}
		}

		if ctx.AsgValidation {
			// Skip nodes which are on unstable ASG
			stable, err := autoScalingGroupIsStable(w, instance.InstanceID)
			if err != nil {
				return err
			}
			if !stable {
				log.Infof("autoscaling-group is in transition, will not reap %v", instance.NodeName)
				continue
			}

			nodesReady, err := ctx.allNodesAreReady()
			if err != nil {
				return err
			}

			// All nodes in the cluster should be in Ready state
			if !nodesReady {
				log.Infof("some nodes in cluster are not ready, skipping OldAge reapable nodes")
				return nil
			}
		}

		// Always Drain
		if ctx.DryRun {
			log.Warnf("dry run is on, '%v' will not be cordon/drained", instance.NodeName)
		}

		var lock LockRecord

		// Dry run does not need to bother with locks
		// and only master nodes need one
		if !ctx.DryRun && isControlPlaneNode && ctx.shouldLock() {
			lock, err = ctx.obtainReapLock(w.DDB, instance.NodeName, instance.InstanceID, controlPlaneType)
			if err != nil {
				// we try to clear the lock is possible, but on failure we skip this node and continue
				// because the lock affects only master nodes
				// TODO: alert on long-lived locks and/or failure to clean up
				ctx.tryClearLock(w.DDB, err, &lock)
				continue
			}
		}

		var controlPlaneCheckError error

		// wrap into a func to make sure defer works as expected
		err = func() error {
			defer func() {
				if lock.Locked() && controlPlaneCheckError == nil {
					// eat the error, the func will log if there's one
					_ = lock.releaseLock(w.DDB)
				}
			}()

			err = ctx.drainNode(instance.NodeName, ctx.DryRun)
			if err != nil {
				return err
			}

			err = dumpSpec(instance.NodeName, ctx.KubernetesClient)
			if err != nil {
				log.Warnf("failed to dump spec for node %v, %v", instance.NodeName, err)
			}

			if !ctx.DryRun {
				log.Infof("reaping old node %v -> %v", instance.NodeName, instance.InstanceID)
				err = ctx.terminateInstance(w.ASG, instance.InstanceID, instance.NodeName)
				if err != nil {
					return err
				}

				// termination call was successful, so we can try to delete the node from the API
				err = ctx.deleteKubernetesNode(instance.NodeName)
				if err != nil {
					log.Warnf("failed to delete the node %v: %v", instance.NodeName, err)
				}

				// Throttle deletion
				ctx.TerminatedInstances++
				ctx.exposeMetric(instance.NodeName, instance.InstanceID, terminationReasonHealthy, NodeReaperResultMetricName, float64(ctx.TerminatedInstances))

				log.Infof("starting deletion throttle wait -> %vs", ctx.AgeReapThrottle)
				time.Sleep(time.Second * time.Duration(ctx.AgeReapThrottle))
			} else {
				log.Warnf("dry run is on, '%v' will not be terminated", instance.NodeName)
			}

			controlPlaneCheckError = ctx.waitForNodesReady()

			// if the control plane did not become healthy in time,
			// the next loop will fail the ready check, so just log the error here
			// the deferred lock release will keep the lock in this case
			if controlPlaneCheckError != nil {
				log.Warnf("error while checking control plane health: %s", controlPlaneCheckError.Error())
			}

			return nil
		}()

		// only return on error, otherwise continue looping
		if err != nil {
			return err
		}
	}
	log.Infof("reap cycle completed, terminated %v instances", ctx.TerminatedInstances)
	return nil
}

func (ctx *ReaperContext) reapUnhealthyNodes(w ReaperAwsAuth) error {
	for _, instance := range ctx.ReapableInstances {

		if ctx.TerminatedInstances >= ctx.MaxKill {
			log.Infof("max kill nodes reached, %v/%v nodes have been terminated in current run", ctx.TerminatedInstances, ctx.MaxKill)
			return nil
		}

		if ctx.AsgValidation && instance.RequiresValidation {
			// Skip nodes which are on unstable ASG
			stable, err := autoScalingGroupIsStable(w, instance.InstanceID)
			if err != nil {
				return err
			}

			if !stable {
				log.Infof("autoscaling-group is in transition, will not reap %v", instance.NodeName)
				continue
			}
		}

		isControlPlaneNode, err := isControlPlane(instance.NodeName, ctx.KubernetesClient)
		if err != nil {
			return err
		}

		var lock LockRecord

		if !ctx.DryRun && isControlPlaneNode && ctx.shouldLock() {
			lock, err = ctx.obtainReapLock(w.DDB, instance.NodeName, instance.InstanceID, controlPlaneType)
			if err != nil {
				// we try to clear the lock is possible, but on failure we skip this node and continue
				// because the lock affects only control plane nodes
				// TODO: alert on long-lived locks and/or failure to clean up (maybe emit metric here)
				ctx.tryClearLock(w.DDB, err, &lock)
				continue
			}
		}

		// wrap into a func to make sure defer works as expected
		err = func() error {
			defer func() {
				if lock.Locked() {
					// eat the error, the func will log if there's one
					_ = lock.releaseLock(w.DDB)
				}
			}()

			// Drain if drainable
			if _, drainable := ctx.DrainableInstances[instance.NodeName]; drainable {
				if ctx.DryRun {
					log.Warnf("dry run is on, '%v' will not be cordon/drained", instance.NodeName)
				}
				err := ctx.drainNode(instance.NodeName, ctx.DryRun)
				if err != nil {
					ctx.exposeMetric(instance.NodeName, instance.InstanceID, drainFailedMetric, NodeReaperResultMetricName, 1)
					return err
				}
			}

			err = dumpSpec(instance.NodeName, ctx.KubernetesClient)
			if err != nil {
				log.Warnf("failed to dump spec for node %v, %v", instance.NodeName, err)
			}

			if !ctx.DryRun {
				log.Infof("reaping unhealthy node %v -> %v", instance.NodeName, instance)

				err = ctx.terminateInstance(w.ASG, instance.InstanceID, instance.NodeName)
				if err != nil {
					return err
				}

				// termination call was successful, so we can try to delete the node from the API
				err = ctx.deleteKubernetesNode(instance.NodeName)
				if err != nil {
					log.Warnf("failed to delete the node %v: %v", instance.NodeName, err)
				}

				// Throttle deletion
				ctx.TerminatedInstances++
				ctx.exposeMetric(instance.NodeName, instance.InstanceID, terminationReasonUnhealthy, NodeReaperResultMetricName, float64(ctx.TerminatedInstances))

				log.Infof("starting deletion throttle wait -> %vs", ctx.ReapThrottle)
				time.Sleep(time.Second * time.Duration(ctx.ReapThrottle))
			} else {
				log.Warnf("dry run is on, '%v' will not be terminated", instance.NodeName)
			}

			return nil
		}()

		// only return on error, otherwise continue looping
		if err != nil {
			return err
		}
	}
	log.Infof("reap cycle completed, terminated %v instances", ctx.TerminatedInstances)
	return nil
}

func (ctx *ReaperContext) scan(w ReaperAwsAuth) error {
	corev1 := ctx.KubernetesClient.CoreV1()

	if ctx.ReapOld {
		if nodeName, ok := os.LookupEnv("NODE_NAME"); ok {
			ctx.SelfNode = nodeName
		}
		if ctx.SelfNode == "" {
			log.Fatalf("failed to get node name, NODE_NAME environment variable is empty or unset")
		}
		log.Infof("Self Node = %v", ctx.SelfNode)
	}

	if podName, ok := os.LookupEnv("POD_NAME"); ok {
		ctx.SelfName = podName
	}
	if ctx.SelfName == "" {
		log.Fatalf("failed to get node name, POD_NAME environment variable is empty or unset")
	}
	log.Infof("Self Pod Name = %v", ctx.SelfName)

	if podNamespace, ok := os.LookupEnv("POD_NAMESPACE"); ok {
		ctx.SelfNamespace = podNamespace
	}
	if ctx.SelfNamespace == "" {
		log.Fatalf("failed to get node name, POD_NAMESPACE environment variable is empty or unset")
	}

	log.Infof("Self Pod Namespace = %v", ctx.SelfNamespace)

	nodeList, err := corev1.Nodes().List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("failed to list all nodes, %v", err)
		return err
	}
	ctx.AllNodes = nodeList.Items

	podList, err := corev1.Pods("").List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("failed to list all pods, %v", err)
		return err
	}
	ctx.AllPods = podList.Items

	eventList, err := corev1.Events("").List(metav1.ListOptions{FieldSelector: "involvedObject.kind=Node"})
	if err != nil {
		log.Errorf("failed to list all events, %v", err)
		return err
	}
	ctx.AllEvents = eventList.Items

	log.Infof("found %v nodes, %v pods, and %v events", len(ctx.AllNodes), len(ctx.AllPods), len(ctx.AllEvents))
	for _, node := range nodeList.Items {
		ctx.NodeInstanceIDs[getNodeInstanceID(&node)] = node.Name
		if nodeStateIsNotReady(&node) || nodeStateIsUnknown(&node) {
			log.Infof("node %v is not ready", node.ObjectMeta.Name)
			ctx.UnreadyNodes = append(ctx.UnreadyNodes, node)
		}
	}

	output, err := w.EC2.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		log.Errorf("failed to list ec2 instances, %v", err)
		return err
	}
	for _, reservation := range output.Reservations {
		for _, instance := range reservation.Instances {
			ctx.AllInstances = append(ctx.AllInstances, instance)
		}
	}

	if ctx.ReapUnjoined {
		describeInput := &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: aws.StringSlice([]string{ctx.ReapUnjoinedKey}),
				},
				{
					Name:   aws.String("tag-value"),
					Values: aws.StringSlice([]string{ctx.ReapUnjoinedValue}),
				},
				{
					Name:   aws.String("instance-state-name"),
					Values: aws.StringSlice([]string{"running"}),
				},
			},
		}
		output, err := w.EC2.DescribeInstances(describeInput)
		if err != nil {
			log.Errorf("failed to list cluster ec2 instances, %v", err)
			return err
		}
		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				ctx.ClusterInstances = append(ctx.ClusterInstances, instance)
				timeSinceLaunch := time.Since(aws.TimeValue(instance.LaunchTime)).Minutes()
				instanceID := aws.StringValue(instance.InstanceId)
				ctx.ClusterInstancesData[instanceID] = timeSinceLaunch
			}
		}

		if len(ctx.ClusterInstances) == 0 {
			err := errors.New("failed to list cluster ec2 instances")
			return err
		}
	}

	return nil
}

func nodeIsAgeReapable(nodeAgeMinutes int, thresholdMinutes int) bool {
	if nodeAgeMinutes >= thresholdMinutes {
		return true
	}
	return false
}

func nodeHasAnnotation(node v1.Node, annotationKey, annotationValue string) bool {
	for k, v := range node.ObjectMeta.Annotations {
		if k == annotationKey && v == annotationValue {
			return true
		}
	}
	return false
}

func autoScalingGroupIsStable(w ReaperAwsAuth, instance string) (bool, error) {
	nodeScalingGroupName, err := getInstanceTagValue(w.EC2, instance, "aws:autoscaling:groupName")
	if err != nil {
		return false, err
	}
	scalingGroup, err := getAutoScalingGroup(w.ASG, nodeScalingGroupName)
	if err != nil {
		return false, err
	}

	var availableInstanceCount int64
	for _, instance := range scalingGroup.Instances {
		if scalingInstanceHealthy(instance) && scalingInstanceInService(instance) {
			availableInstanceCount++
		}
	}

	if aws.Int64Value(scalingGroup.DesiredCapacity) != availableInstanceCount {
		return false, nil
	}

	return true, nil
}

func nodeIsTainted(taint v1.Taint, node v1.Node) bool {
	for _, t := range node.Spec.Taints {
		// ignore timeAdded
		t.TimeAdded = &metav1.Time{Time: time.Time{}}

		// handle key only match
		if taint.Effect == v1.TaintEffect("") && taint.Value == "" && taint.Key == t.Key {
			return true
		}

		if reflect.DeepEqual(taint, t) {
			return true
		}
	}
	return false
}

func nodeIsFlappy(events []v1.Event, name string, threshold int32, reason string) bool {
	totalFlapEvents := make(map[string]int32)
	for _, event := range events {
		eventKind := event.InvolvedObject.Kind
		if eventKind == "Node" {
			nodeName := event.InvolvedObject.Name
			eventCount := event.Count
			eventReason := event.Reason
			if eventReason == reason {
				totalFlapEvents[nodeName] += eventCount
			}
		}
	}
	for node, count := range totalFlapEvents {
		if node == name && count >= threshold {
			log.Infof("node %v has flapped %v/%v times in the last hour", node, count, threshold)
			return true
		}
	}
	return false
}

func hasSkipLabel(node v1.Node, label string) bool {
	return node.ObjectMeta.Labels[reaperDisableLabelKey] == "true" || node.ObjectMeta.Labels[label] == "true"
}

func reconsiderUnreapableNode(node v1.Node, reapableAfter float64) bool {
	//For backward compatibilty
	if nodeHasAnnotation(node, ageUnreapableAnnotationKey, "true") {
		return true
	}

	lastUnreapableTimeStr := getAnnotationValue(node, ageUnreapableAnnotationKey)
	if lastUnreapableTimeStr == "" {
		return true
	}

	lastUnreapableTime, err := time.Parse(time.RFC3339, lastUnreapableTimeStr)

	//invalid date time format
	if err != nil {
		log.Infof("failed to parse age unreapable annotation value: %s", err.Error())
		return false
	}

	now := time.Now().UTC()
	if now.Sub(lastUnreapableTime).Minutes() >= reapableAfter {
		return true
	}

	return false
}

func getAnnotationValue(node v1.Node, annotationKey string) string {
	for k, v := range node.ObjectMeta.Annotations {
		if k == annotationKey {
			return v
		}
	}
	return ""
}

func scalingInstanceHealthy(instance *autoscaling.Instance) bool {
	if aws.StringValue(instance.HealthStatus) == "Healthy" {
		return true
	}
	return false
}

func scalingInstanceInService(instance *autoscaling.Instance) bool {
	if aws.StringValue(instance.LifecycleState) == autoscaling.LifecycleStateInService {
		return true
	}
	return false
}
