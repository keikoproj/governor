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
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

//Context holds the context of the pod-reaper and target cluster
type ReaperContext struct {
	KubernetesClient     kubernetes.Interface
	KubernetesConfigPath string
	SeenPods             *v1.PodList
	StuckPods            map[string]string
	ReapedPods           int
	TimeToReap           float64
	SoftReap             bool
	DryRun               bool
}

// Args is the argument struct for pod-reaper
type Args struct {
	K8sConfigPath string
	ReapAfter     float64
	LocalMode     bool
	SoftReap      bool
	DryRun        bool
}
