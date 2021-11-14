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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/keikoproj/governor/pkg/reaper/common"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/fake"
)

var loggingEnabled bool

func init() {
	flag.BoolVar(&loggingEnabled, "logging-enabled", false, "Enable Reaper Logs")
}

func (m *stubEC2) DescribeTags(input *ec2.DescribeTagsInput) (*ec2.DescribeTagsOutput, error) {
	output := &ec2.DescribeTagsOutput{Tags: []*ec2.TagDescription{
		{
			Key:   aws.String("aws:autoscaling:groupName"),
			Value: aws.String(m.AsgNameTag),
		},
	}}
	return output, nil
}

func (m *stubEC2) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	var tagFilterKey, tagFilterValue, instanceStateFilter, instanceIdFilter, privateDnsFilter string
	var filteredInstances, outputInstances []*ec2.Instance
	outputInstances = m.FakeInstances
	if len(input.Filters) != 0 {
		for _, filter := range input.Filters {
			if aws.StringValue(filter.Name) == "tag-key" {
				tagFilterKey = aws.StringValue(filter.Values[0])
			} else if aws.StringValue(filter.Name) == "tag-value" {
				tagFilterValue = aws.StringValue(filter.Values[0])
			}

			if aws.StringValue(filter.Name) == "instance-state-name" {
				instanceStateFilter = aws.StringValue(filter.Values[0])
			}

			if aws.StringValue(filter.Name) == "instance-id" {
				instanceIdFilter = aws.StringValue(filter.Values[0])
			}

			if aws.StringValue(filter.Name) == "private-dns-name" {
				privateDnsFilter = aws.StringValue(filter.Values[0])
			}
		}

		for _, instance := range m.FakeInstances {
			var match bool
			var matchCount int

			if instanceIdFilter != "" {
				if aws.StringValue(instance.InstanceId) == instanceIdFilter {
					match = true
					matchCount++
				}
			}

			if instanceStateFilter != "" {
				if aws.StringValue(instance.State.Name) == instanceStateFilter {
					match = true
					matchCount++
				}
			}

			if privateDnsFilter != "" {
				if aws.StringValue(instance.PrivateDnsName) == privateDnsFilter {
					match = true
					matchCount++
				}
			}

			for _, tag := range instance.Tags {
				if aws.StringValue(tag.Key) == tagFilterKey && aws.StringValue(tag.Value) == tagFilterValue {
					match = true
					matchCount += 2
				}
			}
			if match && matchCount == len(input.Filters) {
				filteredInstances = append(filteredInstances, instance)
			}
		}

		outputInstances = filteredInstances
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: outputInstances,
			},
		},
	}, nil
}

func (m *stubASG) DescribeAutoScalingGroups(input *autoscaling.DescribeAutoScalingGroupsInput) (*autoscaling.DescribeAutoScalingGroupsOutput, error) {
	instances := []*autoscaling.Instance{}

	for i := 1; i <= int(m.HealthyInstances); i++ {
		instance := &autoscaling.Instance{
			HealthStatus:   aws.String("Healthy"),
			LifecycleState: aws.String(autoscaling.LifecycleStateInService),
		}
		instances = append(instances, instance)
	}

	for i := 1; i <= int(m.UnhealthyInstances); i++ {
		instance := &autoscaling.Instance{
			HealthStatus:   aws.String("Unhealthy"),
			LifecycleState: aws.String(autoscaling.LifecycleStatePending),
		}
		instances = append(instances, instance)
	}

	output := &autoscaling.DescribeAutoScalingGroupsOutput{AutoScalingGroups: []*autoscaling.Group{
		{
			AutoScalingGroupName: aws.String(m.AsgName),
			Instances:            instances,
			DesiredCapacity:      &m.DesiredCapacity,
		},
	}}
	return output, nil
}

func (m *stubASG) TerminateInstanceInAutoScalingGroup(input *autoscaling.TerminateInstanceInAutoScalingGroupInput) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error) {
	output := &autoscaling.TerminateInstanceInAutoScalingGroupOutput{}
	return output, nil
}

func newFakeReaperContext() *ReaperContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}
	os.Setenv("POD_NAME", "node-reaper")
	os.Setenv("POD_NAMESPACE", "governor")
	os.Setenv("NODE_NAME", "self-node.us-west-2.compute.internal")
	ctx := ReaperContext{}
	ctx.KubectlLocalPath = "echo"
	ctx.KubernetesClient = fake.NewSimpleClientset()
	ctx.EC2Region = "us-west-2"
	ctx.ReapThrottle = 0
	ctx.AgeReapThrottle = 0
	ctx.DrainableInstances = make(map[string]string)
	ctx.ReapableInstances = make([]ReapableInstance, 0)
	ctx.GhostInstances = make(map[string]string)
	ctx.ClusterInstancesData = make(map[string]float64)
	ctx.NodeInstanceIDs = make(map[string]string)
	ctx.ReapTainted = make([]v1.Taint, 0)
	ctx.AgeDrainReapableInstances = make([]AgeDrainReapableInstance, 0)
	ctx.AgeKillOrder = make([]string, 0)
	// Default Flags
	ctx.DryRun = false
	ctx.SoftReap = true
	ctx.ReapOld = true
	ctx.ReapUnknown = true
	ctx.ReapUnready = true
	ctx.ReapFlappy = true
	ctx.ReapGhost = true
	ctx.ReapUnjoined = false
	ctx.AsgValidation = true
	ctx.FlapCount = 4
	ctx.TimeToReap = 5
	ctx.ReapOldThresholdMinutes = 36000
	ctx.MaxKill = 3
	ctx.DrainTimeoutSeconds = 600
	ctx.IgnoreFailure = false
	loadFakeAPI(&ctx)
	return &ctx
}

func loadFakeAPI(ctx *ReaperContext) {
	fakeNamespaces := []struct {
		namespaceName string
	}{
		{
			namespaceName: "namespace-1",
		},
		{
			namespaceName: "namespace-2",
		},
		{
			namespaceName: "namespace-3",
		},
	}

	// Create fake namespaces
	for _, c := range fakeNamespaces {
		namespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: c.namespaceName,
		}}
		ctx.KubernetesClient.CoreV1().Namespaces().Create(namespace)
	}
}

func createNodeLabels(node FakeNode, ctx *ReaperContext) map[string]string {
	nodeLabels := make(map[string]string)
	if node.isMaster {
		nodeLabels["kubernetes.io/role"] = "master"
	} else {
		nodeLabels["kubernetes.io/role"] = "node"
	}

	for key, value := range node.nodeLabels {
		nodeLabels[key] = value
	}

	return nodeLabels
}

func createFakeNodes(nodes []FakeNode, ctx *ReaperContext) {
	for _, n := range nodes {
		nodeLabels := createNodeLabels(n, ctx)

		creationTimestamp := metav1.Time{Time: time.Now()}
		if n.ageMinutes != 0 {
			creationTimestamp = metav1.Time{Time: time.Now().Add(time.Duration(-n.ageMinutes) * time.Minute)}
		}

		nodeConditions := []v1.NodeCondition{}

		readyCondition := v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-n.lastTransitionMinutes) * time.Minute)},
			Reason:             "KubeletReady",
			Message:            "kubelet is posting ready status",
		}

		unknownCondition := v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionUnknown,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-n.lastTransitionMinutes) * time.Minute)},
			Reason:             "NodeStatusUnknown",
			Message:            "Kubelet stopped posting node status.",
		}

		notReadyCondition := v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-n.lastTransitionMinutes) * time.Minute)},
			Reason:             "KubeletNotReady",
			Message:            "PLEG is not healthy: pleg was last seen active 9h14m3.5466392s ago; threshold is 3m0",
		}

		switch n.state {
		case "NotReady":
			nodeConditions = append(nodeConditions, notReadyCondition)
		case "Unknown":
			nodeConditions = append(nodeConditions, unknownCondition)
		default:
			nodeConditions = append(nodeConditions, readyCondition)
		}

		if n.providerID == "" {
			n.providerID = "aws:///us-west-2a/i-1a1a12a1a121a12121"
		}

		if n.nodeName == "" {
			n.nodeName = fmt.Sprintf("ip-%v.us-west-2.compute.internal", strings.Replace(randomdata.IpV4Address(), ".", "-", -1))
		}

		fakePods := []FakePod{}
		if n.activePods > 0 {
			for i := 1; i <= n.activePods; i++ {

				fakeActivePod := FakePod{
					podName:       fmt.Sprintf("pod-%v-%v", i, n.nodeName),
					podNamespace:  "namespace-1",
					scheduledNode: n.nodeName,
					isReady:       true,
					nodeLost:      false,
				}

				fakePods = append(fakePods, fakeActivePod)
			}
		}

		if n.unreadyPods > 0 {
			for i := 1; i <= n.unreadyPods; i++ {
				fakeUnreadyPod := FakePod{
					podName:       fmt.Sprintf("pod-%v-%v", i, n.nodeName),
					podNamespace:  "namespace-2",
					scheduledNode: n.nodeName,
					isReady:       false,
					nodeLost:      false,
				}

				fakePods = append(fakePods, fakeUnreadyPod)
			}
		}

		if n.lostPods > 0 {
			for i := 1; i <= n.lostPods; i++ {
				fakeLostPod := FakePod{
					podName:       fmt.Sprintf("pod-%v-%v", i, n.nodeName),
					podNamespace:  "namespace-3",
					scheduledNode: n.nodeName,
					isReady:       false,
					nodeLost:      true,
				}

				fakePods = append(fakePods, fakeLostPod)
			}
		}

		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:              n.nodeName,
			CreationTimestamp: creationTimestamp,
			Labels:            nodeLabels,
		}, Spec: v1.NodeSpec{
			ProviderID: n.providerID,
			Taints:     n.nodeTaints,
		}, Status: v1.NodeStatus{
			Conditions: nodeConditions,
		}}

		ctx.KubernetesClient.CoreV1().Nodes().Create(node)

		for _, c := range fakePods {
			createFakePod(c, ctx)
		}
	}

}

func createFakeEvents(events []FakeEvent, ctx *ReaperContext) {
	for _, e := range events {
		fakeEvent := &v1.Event{
			TypeMeta: metav1.TypeMeta{
				Kind: "Event",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%v.%v", e.node, rand.Int()),
				Namespace: "default",
			},
			InvolvedObject: v1.ObjectReference{
				Kind: e.kind,
				Name: e.node,
			},
			Count:  e.count,
			Reason: e.reason,
		}
		ctx.KubernetesClient.CoreV1().Events("default").Create(fakeEvent)
	}
}

func createFakePod(c FakePod, ctx *ReaperContext) {
	var reasonMsg string

	podConditions := []v1.PodCondition{}

	activeCondition := v1.PodCondition{
		Type:               v1.PodReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-60) * time.Minute)},
	}

	notReadyCondition := v1.PodCondition{
		Type:               v1.PodReady,
		Status:             v1.ConditionFalse,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-60) * time.Minute)},
	}

	if c.isReady {
		podConditions = append(podConditions, activeCondition)
	} else {
		podConditions = append(podConditions, notReadyCondition)
	}

	if c.nodeLost {
		reasonMsg = "NodeLost"
	}
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      c.podName,
		Namespace: c.podNamespace,
	}, Status: v1.PodStatus{
		Conditions: podConditions,
		Reason:     reasonMsg,
	}, Spec: v1.PodSpec{
		NodeName: c.scheduledNode,
	}}
	ctx.KubernetesClient.CoreV1().Pods(c.podNamespace).Create(pod)
}

func runFakeReaper(ctx *ReaperContext, awsAuth ReaperAwsAuth) {
	ctx.scan(awsAuth)
	ctx.deriveFlappyDrainReapableNodes()
	ctx.deriveAgeDrainReapableNodes()
	ctx.deriveGhostDrainReapableNodes(awsAuth)
	ctx.deriveTaintDrainReapableNodes()
	ctx.deriveReapableNodes()
	ctx.reapUnhealthyNodes(awsAuth)
	ctx.reapOldNodes(awsAuth)
}

func createFakeAwsAuth(a FakeASG, i []*ec2.Instance) ReaperAwsAuth {
	awsAuth := ReaperAwsAuth{
		EC2: &stubEC2{AsgNameTag: a.Name, FakeInstances: i},
		ASG: &stubASG{AsgName: a.Name,
			HealthyInstances:   a.Healthy,
			UnhealthyInstances: a.Unhealthy,
			DesiredCapacity:    a.Desired,
		},
	}
	return awsAuth
}

func (u *ReaperUnitTest) Run(t *testing.T, timeTest bool) {
	awsAuth := createFakeAwsAuth(u.InstanceGroup, u.FakeInstances)
	createFakeNodes(u.Nodes, u.FakeReaper)
	createFakeEvents(u.Events, u.FakeReaper)
	start := time.Now()
	runFakeReaper(u.FakeReaper, awsAuth)
	secondsSince := int(time.Since(start).Seconds())

	if timeTest {
		if secondsSince != u.ExpectedDurationSeconds {
			t.Fatalf("expected Duration: %vs, got: %vs", u.ExpectedDurationSeconds, secondsSince)
		}
		return
	}

	if len(u.FakeReaper.UnreadyNodes) != u.ExpectedUnready {
		t.Fatalf("expected Unready: %v, got: %v", u.ExpectedUnready, len(u.FakeReaper.UnreadyNodes))
	}

	if len(u.FakeReaper.DrainableInstances) != u.ExpectedDrainable {
		t.Fatalf("expected Drainable: %v, got: %v", u.ExpectedDrainable, len(u.FakeReaper.DrainableInstances))
	}

	if len(u.FakeReaper.ReapableInstances) != u.ExpectedReapable {
		t.Fatalf("expected Reapable: %v, got: %v", u.ExpectedReapable, len(u.FakeReaper.ReapableInstances))
	}

	if len(u.FakeReaper.AgeDrainReapableInstances) != u.ExpectedOldReapable {
		t.Fatalf("expected Age Reapable: %v, got: %v", u.ExpectedOldReapable, len(u.FakeReaper.AgeDrainReapableInstances))
	}

	if u.FakeReaper.DrainedInstances != u.ExpectedDrained {
		t.Fatalf("expected Drained: %v, got: %v", u.ExpectedDrained, u.FakeReaper.DrainedInstances)
	}

	if u.FakeReaper.TerminatedInstances != u.ExpectedTerminated {
		t.Fatalf("expected Terminated: %v, got: %v", u.ExpectedTerminated, u.FakeReaper.TerminatedInstances)
	}
	if len(u.ExpectedKillOrder) != 0 {
		if !reflect.DeepEqual(u.FakeReaper.AgeKillOrder, u.ExpectedKillOrder) {
			t.Fatalf("expected KillOrder: %v, got: %v", u.ExpectedKillOrder, u.FakeReaper.AgeKillOrder)
		}
	}
}

type ReaperUnitTest struct {
	TestDescription         string
	Nodes                   []FakeNode
	Events                  []FakeEvent
	InstanceGroup           FakeASG
	FakeReaper              *ReaperContext
	FakeInstances           []*ec2.Instance
	ExpectedTerminated      int
	ExpectedDrained         int
	ExpectedReapable        int
	ExpectedOldReapable     int
	ExpectedUnready         int
	ExpectedDrainable       int
	ExpectedDurationSeconds int
	ExpectedKillOrder       []string
}

type FakeASG struct {
	Name      string
	Healthy   int64
	Desired   int64
	Unhealthy int64
}

type FakeNode struct {
	nodeName              string
	state                 string
	providerID            string
	lastTransitionMinutes int64
	isMaster              bool
	activePods            int
	unreadyPods           int
	lostPods              int
	ageMinutes            int
	nodeTaints            []v1.Taint
	nodeLabels            map[string]string
}
type FakePod struct {
	podName       string
	podNamespace  string
	isReady       bool
	nodeLost      bool
	scheduledNode string
}

type FakeEvent struct {
	count  int32
	reason string
	node   string
	kind   string
}

type stubEC2 struct {
	ec2iface.EC2API
	AsgNameTag    string
	FakeInstances []*ec2.Instance
}

type stubASG struct {
	autoscalingiface.AutoScalingAPI
	AsgName            string
	HealthyInstances   int64
	UnhealthyInstances int64
	DesiredCapacity    int64
}

func TestReaperContext_drainNode(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.IgnoreFailure = true
	type args struct {
		name   string
		dryRun bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name:    "node with ignore failure true",
			args: args{"temp", false},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := reaper.drainNode(tt.args.name, tt.args.dryRun); (err != nil) != tt.wantErr {
				t.Errorf("drainNode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetUnreadyNodesPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Derive Readiness - should find NotReady & Unknown nodes",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 2,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state: "NotReady",
			},
			{
				state: "Unknown",
			},
		},
		FakeReaper:      reaper,
		ExpectedUnready: 2,
	}
	testCase.Run(t, false)
}

func TestGetUnreadyNodesNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Derive Readiness - should not find Ready nodes",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state: "Ready",
			},
		},
		FakeReaper:      reaper,
		ExpectedUnready: 0,
	}
	testCase.Run(t, false)
}

func TestReapOldPositive(t *testing.T) {
	reaper := newFakeReaperContext()

	testCase := ReaperUnitTest{
		TestDescription: "Reap Old - should reap healthy nodes older than N days",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   4,
			Unhealthy: 0,
			Desired:   4,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-1",
				state:      "Ready",
				ageMinutes: 43100,
			},
			{
				nodeName:   "node-2",
				state:      "Ready",
				ageMinutes: 43000,
			},
			{
				nodeName:   "node-3",
				state:      "Ready",
				ageMinutes: 43200,
			},
			{
				nodeName:   "node-4",
				state:      "Ready",
				ageMinutes: 60,
			},
		},
		FakeReaper:          reaper,
		ExpectedOldReapable: 3,
		ExpectedTerminated:  3,
		ExpectedDrained:     3,
		ExpectedKillOrder:   []string{"node-3", "node-1", "node-2"},
	}
	testCase.Run(t, false)
}

func TestReapTaintedPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapTainted = []v1.Taint{
		{
			Key:       "key",
			Value:     "value",
			Effect:    "effect",
			TimeAdded: &metav1.Time{Time: time.Time{}},
		},
		{
			Key:       "key",
			Effect:    "effect",
			TimeAdded: &metav1.Time{Time: time.Time{}},
		},
		{
			Key:       "key",
			TimeAdded: &metav1.Time{Time: time.Time{}},
		},
	}

	testCase := ReaperUnitTest{
		TestDescription: "Reap Tainted - should reap tainted nodes",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   4,
			Unhealthy: 0,
			Desired:   4,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-1",
				state:      "Ready",
				ageMinutes: 60,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Value:     "value",
						Effect:    "effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-2",
				state:      "Ready",
				ageMinutes: 120,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Effect:    "effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-3",
				state:      "Ready",
				ageMinutes: 240,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Effect:    "some-effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-4",
				state:      "Ready",
				ageMinutes: 10,
			},
		},
		FakeReaper:         reaper,
		ExpectedReapable:   3,
		ExpectedTerminated: 3,
		ExpectedDrainable:  3,
		ExpectedDrained:    3,
	}
	testCase.Run(t, false)
}

func TestReapTaintedNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapTainted = []v1.Taint{}

	testCase := ReaperUnitTest{
		TestDescription: "Reap Tainted - should not reap tainted nodes if taints not provided",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   4,
			Unhealthy: 0,
			Desired:   4,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-1",
				state:      "Ready",
				ageMinutes: 60,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Value:     "value",
						Effect:    "effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-2",
				state:      "Ready",
				ageMinutes: 120,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Effect:    "effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-3",
				state:      "Ready",
				ageMinutes: 240,
				nodeTaints: []v1.Taint{
					{
						Key:       "key",
						Effect:    "some-effect",
						TimeAdded: &metav1.Time{Time: time.Time{}},
					},
				},
			},
			{
				nodeName:   "node-4",
				state:      "Ready",
				ageMinutes: 60,
			},
		},
		FakeReaper:         reaper,
		ExpectedReapable:   0,
		ExpectedTerminated: 0,
		ExpectedDrained:    0,
	}
	testCase.Run(t, false)
}

func TestReapOldNegative(t *testing.T) {
	reaper := newFakeReaperContext()

	testCase := ReaperUnitTest{
		TestDescription: "Reap Old - old nodes should not be reaped when some nodes are NotReady",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
			{
				state:      "NotReady",
				ageMinutes: 60,
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     1,
		ExpectedOldReapable: 1,
		ExpectedTerminated:  0,
		ExpectedDrained:     0,
	}
	testCase.Run(t, false)
}

func TestReapOldDisabled(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapOld = false

	testCase := ReaperUnitTest{
		TestDescription: "Reap Old - old nodes should not be reaped when switched off",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     0,
		ExpectedOldReapable: 0,
		ExpectedTerminated:  0,
		ExpectedDrained:     0,
	}
	testCase.Run(t, false)
}

func TestIgnoreReapFailure(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.IgnoreFailure = true
	testCase := ReaperUnitTest{
		TestDescription: "Ignore failure - old nodes should be cordoned",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:      "Unknown",
				ageMinutes: 43200,
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     1,
		ExpectedOldReapable: 1,
		ExpectedTerminated:  0,
		ExpectedDrained:     0,
	}
	testCase.Run(t, false)
}

func TestReapOldSelfEviction(t *testing.T) {
	reaper := newFakeReaperContext()

	testCase := ReaperUnitTest{
		TestDescription: "Reap Old - old nodes must skip if scheduled to self node",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "self-node.us-west-2.compute.internal",
				state:      "Ready",
				ageMinutes: 43200,
			},
			{
				state:      "Ready",
				ageMinutes: 60,
			},
		},
		FakeReaper:          reaper,
		ExpectedOldReapable: 1,
		ExpectedTerminated:  0,
		ExpectedDrained:     0,
	}
	testCase.Run(t, false)
}

func TestReapUnknownPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = true
	reaper.AsgValidation = false

	testCase := ReaperUnitTest{
		TestDescription: "Reap Unknown - should reap unknown nodes",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:                 "Unknown",
				lastTransitionMinutes: 6,
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    1,
		ExpectedReapable:   1,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestReapUnknownNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = true
	reaper.ReapUnready = false

	testCase := ReaperUnitTest{
		TestDescription: "Reap Unknown - should not reap nodes which are Ready & NotReady",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state: "NotReady",
			},
			{
				state: "Ready",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    1,
		ExpectedReapable:   0,
		ExpectedTerminated: 0,
	}
	testCase.Run(t, false)
}

func TestReapUnreadyPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = false
	reaper.AsgValidation = false

	testCase := ReaperUnitTest{
		TestDescription: "Reap Unready - should reap unready nodes",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    1,
		ExpectedReapable:   1,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestReapUnreadyNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = false
	reaper.ReapUnready = true

	testCase := ReaperUnitTest{
		TestDescription: "Reap Unready - should not reap nodes which are Ready & Unknown",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state: "Unknown",
			},
			{
				state: "Ready",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    1,
		ExpectedReapable:   0,
		ExpectedTerminated: 0,
	}
	testCase.Run(t, false)
}

func TestFlapDetectionPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.FlapCount = 4

	testCase := ReaperUnitTest{
		TestDescription: "Flap Detection - flappy nodes should drain-reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				nodeName: "ip-10-10-10-10.us-west-2.compute.local",
				state:    "Ready",
			},
		},
		Events: []FakeEvent{
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  3,
				reason: "NodeReady",
				kind:   "Node",
			},
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  1,
				reason: "NodeReady",
				kind:   "Node",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   1,
		ExpectedDrainable:  1,
		ExpectedTerminated: 1,
		ExpectedDrained:    1,
	}
	testCase.Run(t, false)
}

func TestFlapDetectionNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.FlapCount = 4

	testCase := ReaperUnitTest{
		TestDescription: "Flap Detection - non-flappy nodes should not be drained or reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				nodeName: "ip-10-10-10-10.us-west-2.compute.local",
				state:    "Ready",
			},
		},
		Events: []FakeEvent{
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  3,
				reason: "NodeReady",
				kind:   "Node",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   0,
		ExpectedDrainable:  0,
		ExpectedTerminated: 0,
		ExpectedDrained:    0,
	}
	testCase.Run(t, false)
}

func TestAsgValidationPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = true

	testCase := ReaperUnitTest{
		TestDescription: "ASG Validation - should validate ASG as a condition to reap",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 1,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:                 "NotReady",
				lastTransitionMinutes: 10,
			},
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
		},
		FakeReaper:          reaper,
		ExpectedOldReapable: 1,
		ExpectedUnready:     1,
		ExpectedReapable:    1,
		ExpectedTerminated:  0,
	}
	testCase.Run(t, false)
}

func TestAsgValidationNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false

	testCase := ReaperUnitTest{
		TestDescription: "ASG Validation - should not validate ASG as a condition to reap",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 1,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:                 "NotReady",
				lastTransitionMinutes: 10,
			},
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     1,
		ExpectedReapable:    1,
		ExpectedDrained:     1,
		ExpectedOldReapable: 1,
		ExpectedTerminated:  2,
	}
	testCase.Run(t, false)
}
func TestSoftReapPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false

	testCase := ReaperUnitTest{
		TestDescription: "Soft Reap - nodes with active pods are not reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:                 "Unknown",
				lastTransitionMinutes: 6,
				lostPods:              1,
				activePods:            1,
			},
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
				activePods:            2,
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   0,
		ExpectedTerminated: 0,
	}
	testCase.Run(t, false)
}

func TestSoftReapNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false
	reaper.SoftReap = false

	testCase := ReaperUnitTest{
		TestDescription: "Soft Reap - nodes with active pods are reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:                 "Unknown",
				lastTransitionMinutes: 6,
				activePods:            2,
			},
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
				activePods:            2,
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   2,
		ExpectedTerminated: 2,
	}
	testCase.Run(t, false)
}

func TestReapThrottleWaiter(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false
	reaper.ReapThrottle = 2

	testCase := ReaperUnitTest{
		TestDescription: "Reap Throttle - reaper should wait after each reaping",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 1,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
			},
		},
		FakeReaper:              reaper,
		ExpectedUnready:         1,
		ExpectedReapable:        1,
		ExpectedTerminated:      1,
		ExpectedDurationSeconds: 2,
	}
	testCase.Run(t, true)
}

func TestAgeReapThrottleWaiter(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapThrottle = 30
	reaper.AgeReapThrottle = 1

	testCase := ReaperUnitTest{
		TestDescription: "Age Reap Throttle - reaper should wait after each old node reaping",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
			{
				state:      "Ready",
				ageMinutes: 43200,
			},
		},
		FakeReaper:              reaper,
		ExpectedReapable:        2,
		ExpectedDrained:         2,
		ExpectedTerminated:      2,
		ExpectedDurationSeconds: 2,
	}
	testCase.Run(t, true)
}

func TestDryRun(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false
	reaper.DryRun = true

	testCase := ReaperUnitTest{
		TestDescription: "Dry Run - nodes should not be terminated",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 2,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName: "ip-10-10-10-10.us-west-2.compute.local",
				state:    "Ready",
			},
			{
				state:                 "Unknown",
				lastTransitionMinutes: 6,
			},
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
			},
		},
		Events: []FakeEvent{
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  30,
				reason: "NodeReady",
				kind:   "Node",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   3,
		ExpectedDrainable:  1,
		ExpectedDrained:    0,
		ExpectedTerminated: 0,
	}
	testCase.Run(t, false)
}

func TestKillOldMasterMinMasters(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.MaxKill = 1
	reaper.ReapOld = true

	testCase := ReaperUnitTest{
		TestDescription: "Old Masters - old masters should be terminated only if there is a minimum of 3 healthy masters",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   3,
			Unhealthy: 0,
			Desired:   3,
		},
		Nodes: []FakeNode{
			{
				state:      "Ready",
				isMaster:   true,
				ageMinutes: 43200,
			},
			{
				state:      "Ready",
				isMaster:   true,
				ageMinutes: 43200,
			},
			{
				state:      "Ready",
				isMaster:   true,
				ageMinutes: 10,
			},
		},
		FakeReaper:          reaper,
		ExpectedOldReapable: 2,
		ExpectedDrained:     1,
		ExpectedTerminated:  1,
	}
	testCase.Run(t, false)
}

func TestMaxKill(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.MaxKill = 1

	testCase := ReaperUnitTest{
		TestDescription: "Max Kill - nodes should be terminated up to max kill limit",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
			},
			{
				state:                 "NotReady",
				lastTransitionMinutes: 6,
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   2,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestUnjoinedPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnjoined = true
	reaper.ReapUnjoinedThresholdMinutes = 15
	reaper.ReapUnjoinedKey = "KubernetesCluster"
	reaper.ReapUnjoinedValue = "my-cluster"

	testCase := ReaperUnitTest{
		TestDescription: "Unjoined - nodes should be terminated if they are unjoined",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-10-10-10-10",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-101010101010",
			},
			{
				nodeName:   "node-20-20-20-20",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-202020202020",
			},
		},
		FakeInstances: []*ec2.Instance{
			{
				InstanceId: aws.String("i-303030303030"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-30) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("my-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
			{
				InstanceId: aws.String("i-404040404040"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-30) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("different-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   1,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestUnjoinedValidation(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnjoined = true
	reaper.ReapUnjoinedThresholdMinutes = 15
	reaper.AsgValidation = true
	reaper.ReapUnjoinedKey = "KubernetesCluster"
	reaper.ReapUnjoinedValue = "my-cluster"

	testCase := ReaperUnitTest{
		TestDescription: "Unjoined - nodes should be terminated if they are unjoined",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   0,
			Unhealthy: 2,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-10-10-10-10",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-101010101010",
			},
			{
				nodeName:   "node-20-20-20-20",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-202020202020",
			},
		},
		FakeInstances: []*ec2.Instance{
			{
				InstanceId: aws.String("i-303030303030"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-30) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("my-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
			{
				InstanceId: aws.String("i-404040404040"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-30) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("different-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   1,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestUnjoinedNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnjoined = true
	reaper.ReapUnjoinedThresholdMinutes = 15
	reaper.ReapUnjoinedKey = "KubernetesCluster"
	reaper.ReapUnjoinedValue = "my-cluster"

	testCase := ReaperUnitTest{
		TestDescription: "Unjoined - nodes should not be terminated if they are unjoined but do not meet threshold",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-10-10-10-10",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-101010101010",
			},
			{
				nodeName:   "node-20-20-20-20",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-303030303030",
			},
		},
		FakeInstances: []*ec2.Instance{
			{
				InstanceId: aws.String("i-303030303030"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-30) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("my-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
			{
				InstanceId: aws.String("i-404040404040"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-10) * time.Minute)),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("KubernetesCluster"),
						Value: aws.String("my-cluster"),
					},
				},
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   0,
		ExpectedTerminated: 0,
	}
	testCase.Run(t, false)
}

func TestGhostPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapGhost = true
	testCase := ReaperUnitTest{
		TestDescription: "Ghost - nodes should be removed if they referencing a termianted instance-id",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-10-10-10-10",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-101010101010",
			},
			{
				nodeName:   "node-20-20-20-20",
				state:      "Ready",
				providerID: "aws:///us-west-2a/i-202020202020",
			},
		},
		FakeInstances: []*ec2.Instance{
			{
				InstanceId: aws.String("i-101010101010"),
				LaunchTime: aws.Time(time.Now().Add(time.Duration(-100) * time.Minute)),
				State: &ec2.InstanceState{
					Name: aws.String("terminated"),
				},
			},
			{
				PrivateDnsName: aws.String("node-10-10-10-10"),
				InstanceId:     aws.String("i-303030303030"),
				LaunchTime:     aws.Time(time.Now().Add(time.Duration(-10) * time.Minute)),
				State: &ec2.InstanceState{
					Name: aws.String("running"),
				},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedDrainable:  1,
		ExpectedDrained:    1,
		ExpectedReapable:   1,
		ExpectedTerminated: 1,
	}
	testCase.Run(t, false)
}

func TestProviderIDParser(t *testing.T) {
	// TestDescription: ProviderID can be parsed to get Region and InstanceID
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false
	reaper.DryRun = true

	node := v1.Node{
		Spec: v1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
		},
	}

	providerRegion := getNodeRegion(&node)
	providerInstanceID := getNodeInstanceID(&node)
	expectedInstanceID := "i-1234567890abcdef0"
	expectedRegion := "us-west-2"
	if providerInstanceID != expectedInstanceID {
		t.Fatalf("expected InstanceID: %v, got: %v", expectedInstanceID, providerInstanceID)
	}
	if providerRegion != expectedRegion {
		t.Fatalf("expected Region: %v, got: %v", expectedRegion, providerRegion)
	}
}

func TestRegionDetection(t *testing.T) {
	// TestDescription: Region value in annotation should take priority over providerId
	reaper := newFakeReaperContext()
	reaper.AsgValidation = false
	reaper.DryRun = true

	node := v1.Node{
		Spec: v1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
		},
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"topology.kubernetes.io/region": "us-east-2",
		}},
	}

	providerRegion := getNodeRegion(&node)
	expectedRegion := "us-east-2"

	if providerRegion != expectedRegion {
		t.Fatalf("expected Region: %v, got: %v", expectedRegion, providerRegion)
	}
}


func TestSkipLabelReaper(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = true
	reaper.ReapUnready = true
	reaper.AsgValidation = true
	reaper.FlapCount = 4

	testCase := ReaperUnitTest{
		TestDescription: "DisableReaper label detection - skip reaping of any node if it has this label with value 'true'",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-old",
				state:      "Ready",
				ageMinutes: 43100,
				nodeLabels: map[string]string{reaperDisableLabelKey: "true"},
			},
			{
				nodeName:   "node-flappy",
				state:      "Ready",
				nodeLabels: map[string]string{reaperDisableLabelKey: "true"},
			},
			{
				nodeName:              "node-unknown",
				state:                 "Unknown",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reaperDisableLabelKey: "true"},
			},
			{
				nodeName:              "node-unready",
				state:                 "NotReady",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reaperDisableLabelKey: "true"},
			},
		},
		Events: []FakeEvent{
			{
				node:   "node-flappy",
				count:  4,
				reason: "NodeReady",
				kind:   "Node",
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     2,
		ExpectedReapable:    0,
		ExpectedDrainable:   0,
		ExpectedOldReapable: 0,
		ExpectedTerminated:  0,
		ExpectedDrained:     0,
	}
	testCase.Run(t, false)
}

func TestSkipLabelUnknownNodes(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnknown = true
	reaper.AsgValidation = true

	testCase := ReaperUnitTest{
		TestDescription: "DisableUnknownReaper label Detection - unknown nodes with skip label as 'true' are skipped from being reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				nodeName:              "node-unknown-1",
				state:                 "Unknown",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reapUnknownDisabledLabelKey: "true"},
			},
			{
				nodeName:              "node-unknown-2",
				state:                 "Unknown",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reapUnknownDisabledLabelKey: "false"},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   1,
		ExpectedDrainable:  0,
		ExpectedTerminated: 1,
		ExpectedDrained:    0,
	}
	testCase.Run(t, false)
}

func TestSkipLabelUnreadyNodes(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.ReapUnready = true
	reaper.AsgValidation = true

	testCase := ReaperUnitTest{
		TestDescription: "DisableUnreadyReaper label Detection - unready nodes with skip label as 'true' are skipped from being reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   1,
			Unhealthy: 0,
			Desired:   1,
		},
		Nodes: []FakeNode{
			{
				nodeName:              "node-unready-1",
				state:                 "NotReady",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reapUnreadyDisabledLabelKey: "true"},
			},
			{
				nodeName:              "node-unready-2",
				state:                 "NotReady",
				lastTransitionMinutes: 6,
				nodeLabels:            map[string]string{reapUnreadyDisabledLabelKey: "false"},
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    2,
		ExpectedReapable:   1,
		ExpectedDrainable:  0,
		ExpectedTerminated: 1,
		ExpectedDrained:    0,
	}
	testCase.Run(t, false)
}

func TestSkipLabelOldNodes(t *testing.T) {
	reaper := newFakeReaperContext()

	testCase := ReaperUnitTest{
		TestDescription: "DisableOldReaper label Detection - old nodes with skip label as 'true' are skipped from being reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "node-old-1",
				state:      "Ready",
				ageMinutes: 43100,
				nodeLabels: map[string]string{reapOldDisabledLabelKey: "true"},
			},
			{
				nodeName:   "node-old-2",
				state:      "Ready",
				ageMinutes: 43100,
				nodeLabels: map[string]string{reapOldDisabledLabelKey: "false"},
			},
		},
		FakeReaper:          reaper,
		ExpectedUnready:     0,
		ExpectedOldReapable: 1,
		ExpectedTerminated:  1,
		ExpectedDrained:     1,
	}
	testCase.Run(t, false)
}

func TestSkipLabelFlappyNodes(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.FlapCount = 4

	testCase := ReaperUnitTest{
		TestDescription: "DisableFlappyReaper label Detection - flappy nodes with skip label as 'true' are skipped from being reaped",
		InstanceGroup: FakeASG{
			Name:      "my-ig.cluster.k8s.local",
			Healthy:   2,
			Unhealthy: 0,
			Desired:   2,
		},
		Nodes: []FakeNode{
			{
				nodeName:   "ip-10-10-10-10.us-west-2.compute.local",
				state:      "Ready",
				nodeLabels: map[string]string{reapFlappyDisabledLabelKey: "true"},
			},
			{
				nodeName:   "ip-10-10-10-11.us-west-2.compute.local",
				state:      "Ready",
				nodeLabels: map[string]string{reapFlappyDisabledLabelKey: "false"},
			},
		},
		Events: []FakeEvent{
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  3,
				reason: "NodeReady",
				kind:   "Node",
			},
			{
				node:   "ip-10-10-10-10.us-west-2.compute.local",
				count:  1,
				reason: "NodeReady",
				kind:   "Node",
			},
			{
				node:   "ip-10-10-10-11.us-west-2.compute.local",
				count:  4,
				reason: "NodeReady",
				kind:   "Node",
			},
		},
		FakeReaper:         reaper,
		ExpectedUnready:    0,
		ExpectedReapable:   1,
		ExpectedDrainable:  1,
		ExpectedTerminated: 1,
		ExpectedDrained:    1,
	}
	testCase.Run(t, false)

}

func TestParseTaint(t *testing.T) {

	fullTaint := v1.Taint{
		Key:       "fullTaint",
		Value:     "value",
		Effect:    "NoSchedule",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	keyEffectTaint := v1.Taint{
		Key:       "keyEffectTaint",
		Effect:    "NoSchedule",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	keyTaint := v1.Taint{
		Key:       "keyTaint",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	emptyTaint := v1.Taint{
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	tests := []struct {
		taintStr      string
		expectedTaint v1.Taint
	}{
		{taintStr: "fullTaint=value:NoSchedule", expectedTaint: fullTaint},
		{taintStr: "keyEffectTaint:NoSchedule", expectedTaint: keyEffectTaint},
		{taintStr: "keyTaint", expectedTaint: keyTaint},
		{taintStr: "invalid:taint:invalid=taint", expectedTaint: v1.Taint{}},
		{taintStr: "invalid=taint=invalid:taint", expectedTaint: v1.Taint{}},
		{taintStr: "", expectedTaint: emptyTaint},
	}

	for i, tc := range tests {
		got, _, _ := parseTaint(tc.taintStr)
		if !reflect.DeepEqual(got, tc.expectedTaint) {
			t.Fatalf("test #%v: expected match: %+v, got: %+v", i, tc.expectedTaint, got)
		}
	}
}

func TestNodeIsTainted(t *testing.T) {
	fullTaint := v1.Taint{
		Key:       "fullTaint",
		Value:     "value",
		Effect:    "NoSchedule",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	keyEffectTaint := v1.Taint{
		Key:       "keyEffectTaint",
		Effect:    "NoSchedule",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	keyTaint := v1.Taint{
		Key:       "keyTaint",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	otherTaint := v1.Taint{
		Key:       "otherTaint",
		Value:     "value",
		Effect:    "NoSchedule",
		TimeAdded: &metav1.Time{Time: time.Time{}},
	}

	taintedNode := v1.Node{
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{fullTaint, keyEffectTaint, keyTaint},
		},
	}

	tests := []struct {
		taint       v1.Taint
		node        v1.Node
		shouldMatch bool
	}{
		{taint: fullTaint, node: taintedNode, shouldMatch: true},
		{taint: keyEffectTaint, node: taintedNode, shouldMatch: true},
		{taint: keyTaint, node: taintedNode, shouldMatch: true},
		{taint: otherTaint, node: taintedNode, shouldMatch: false},
		{taint: otherTaint, node: v1.Node{}, shouldMatch: false},
	}

	for i, tc := range tests {
		got := nodeIsTainted(tc.taint, tc.node)
		if !reflect.DeepEqual(got, tc.shouldMatch) {
			t.Fatalf("test #%v: expected match: %t, got: %t", i, tc.shouldMatch, got)
		}
	}
}

func TestReconsiderUnreapableNode(t *testing.T) {
	testCases := []struct {
		Name                string
		Node                v1.Node
		ReapableAfter       float64
		ReconsideUnreapable bool
	}{
		{
			Name: "node without age unreapable annotation",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			ReapableAfter:       10,
			ReconsideUnreapable: true,
		},
		{
			Name: "node with age unreapable set to true",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ageUnreapableAnnotationKey: "true",
					},
				},
			},
			ReapableAfter:       10,
			ReconsideUnreapable: true,
		},
		{
			Name: "node with over age unreapable date time",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ageUnreapableAnnotationKey: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					},
				},
			},
			ReapableAfter:       10,
			ReconsideUnreapable: true,
		},
		{
			Name: "node with under age unreapable date time",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ageUnreapableAnnotationKey: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
					},
				},
			},
			ReapableAfter:       10,
			ReconsideUnreapable: false,
		},
		{
			Name: "node with invalid age unreapable date time format",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ageUnreapableAnnotationKey: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC1123),
					},
				},
			},
			ReapableAfter:       10,
			ReconsideUnreapable: false,
		},
	}

	for _, tc := range testCases {
		got := reconsiderUnreapableNode(tc.Node, tc.ReapableAfter)
		if got != tc.ReconsideUnreapable {
			t.Fatalf("test #%v: expected match: %t, got: %t", tc.Name, tc.ReconsideUnreapable, got)
		}
	}
}

func TestGetAnnotationValue(t *testing.T) {
	testCases := []struct {
		Name          string
		Node          v1.Node
		AnnotationKey string
		ExpectedValue string
	}{
		{
			Name: "invalid key",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			AnnotationKey: "key",
			ExpectedValue: "",
		},
		{
			Name: "with valid annotation key value",
			Node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key": "value",
					},
				},
			},
			AnnotationKey: "key",
			ExpectedValue: "value",
		},
	}

	for _, tc := range testCases {
		got := getAnnotationValue(tc.Node, tc.AnnotationKey)
		if got != tc.ExpectedValue {
			t.Fatalf("test #%v: expected match: %s, got: %s", tc.Name, tc.ExpectedValue, got)
		}
	}
}
