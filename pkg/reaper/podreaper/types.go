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

package podreaper

import (
	"time"

	"github.com/keikoproj/governor/pkg/reaper/common"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// ReaperContext holds the context of the pod-reaper and target cluster
type ReaperContext struct {
	KubernetesClient     kubernetes.Interface
	KubernetesConfigPath string
	TerminatingPods      *v1.PodList
	AllPods              *v1.PodList
	AllNamespaces        *v1.NamespaceList
	StuckPods            map[string]string
	CompletedPods        map[string]string
	FailedPods           map[string]string
	ReapedPods           int
	TimeToReap           float64
	ReapCompletedAfter   float64
	ReapFailedAfter      float64
	ReapCompleted        bool
	ReapFailed           bool
	SoftReap             bool
	DryRun               bool
	MetricsAPI           common.MetricsAPI
}

// Args is the argument struct for pod-reaper
type Args struct {
	K8sConfigPath      string
	ReapAfter          float64
	LocalMode          bool
	SoftReap           bool
	DryRun             bool
	ReapCompleted      bool
	ReapCompletedAfter float64
	ReapFailed         bool
	ReapFailedAfter    float64
	PromPushgateway    string
}

type FinishTimes []time.Time

func (p FinishTimes) Len() int {
	return len(p)
}

func (p FinishTimes) Less(i, j int) bool {
	return p[i].Before(p[j])
}

func (p FinishTimes) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
