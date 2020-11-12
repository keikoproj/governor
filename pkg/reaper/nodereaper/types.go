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
	"sort"

	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// ReaperAwsAuth is an AWS client-set
type ReaperAwsAuth struct {
	EC2 ec2iface.EC2API
	ASG autoscalingiface.AutoScalingAPI
}

// Args is the argument struct for node-reaper
type Args struct {
	K8sConfigPath                string
	ReaperConfigFilePath         string
	KubectlLocalPath             string
	EC2Region                    string
	ReapUnjoinedKey              string
	ReapUnjoinedValue            string
	DryRun                       bool
	SoftReap                     bool
	LocalMode                    bool
	ReapUnknown                  bool
	ReapUnready                  bool
	ReapGhost                    bool
	ReapUnjoined                 bool
	ReapFlappy                   bool
	AsgValidation                bool
	ReapOld                      bool
	FlapCount                    int32
	ReapOldThresholdMinutes      int32
	ReapUnjoinedThresholdMinutes int32
	MaxKill                      int
	ReapThrottle                 int64
	AgeReapThrottle              int64
	ReapAfter                    float64
	ReapTainted                  []string
	ReconsiderUnreapableAfter    float64
}

// ReaperContext holds the context of the node-reaper and target cluster
type ReaperContext struct {
	// clients
	KubernetesClient     kubernetes.Interface
	KubernetesConfigPath string
	// validated arguments
	ReaperConfigFilePath         string
	EC2Region                    string
	KubectlLocalPath             string
	ReapUnjoinedKey              string
	ReapUnjoinedValue            string
	DryRun                       bool
	SoftReap                     bool
	ReapUnknown                  bool
	ReapUnready                  bool
	ReapGhost                    bool
	ReapUnjoined                 bool
	ReapFlappy                   bool
	AsgValidation                bool
	ReapOld                      bool
	ReapThrottle                 int64
	AgeReapThrottle              int64
	ReapOldThresholdMinutes      int32
	ReapUnjoinedThresholdMinutes int32
	FlapCount                    int32
	MaxKill                      int
	TimeToReap                   float64
	ReapTainted                  []v1.Taint
	ReconsiderUnreapableAfter    float64
	// runtime
	UnreadyNodes              []v1.Node
	AllNodes                  []v1.Node
	AllPods                   []v1.Pod
	AllEvents                 []v1.Event
	AllInstances              []*ec2.Instance
	ClusterInstances          []*ec2.Instance
	ClusterInstancesData      map[string]float64
	GhostInstances            map[string]string
	NodeInstanceIDs           map[string]string
	SelfNode                  string
	SelfNamespace             string
	SelfName                  string
	AgeKillOrder              []string
	AgeDrainReapableInstances []AgeDrainReapableInstance
	ReapableInstances         map[string]string
	DrainableInstances        map[string]string
	TerminatedInstances       int
	DrainedInstances          int
}

// AgeDrainReapableInstances holds an age-reapable node
type AgeDrainReapableInstance struct {
	NodeName   string
	InstanceID string
	AgeMinutes int
}

// AgeSorter sorts age-reapable nodes by their AgeMinutes
type AgeSorter []AgeDrainReapableInstance

func (a AgeSorter) Len() int           { return len(a) }
func (a AgeSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a AgeSorter) Less(i, j int) bool { return a[i].AgeMinutes > a[j].AgeMinutes }

func (ctx *ReaperContext) addReapable(name string, id string) {
	ctx.ReapableInstances[name] = id
}

func (ctx *ReaperContext) addDrainable(name string, id string) {
	ctx.DrainableInstances[name] = id
}

// Different queue for age-reapable in order to allow for different throttle / validation checks
func (ctx *ReaperContext) addAgeDrainReapable(name string, id string, age int) {
	node := AgeDrainReapableInstance{
		NodeName:   name,
		InstanceID: id,
		AgeMinutes: age,
	}
	ctx.AgeDrainReapableInstances = append(ctx.AgeDrainReapableInstances, node)
	// Sort by age after adding a new object
	sort.Sort(AgeSorter(ctx.AgeDrainReapableInstances))
}
