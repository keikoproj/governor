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

package pdbreaper

import (
	"io/ioutil"
	"testing"

	"github.com/keikoproj/governor/pkg/reaper/common"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeReaperContext() *ReaperContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}
	ctx := ReaperContext{}
	ctx.KubernetesClient = fake.NewSimpleClientset()
	// Default Flags
	ctx.DryRun = false
	return &ctx
}

func TestReaperContext_validate(t *testing.T) {
	// type fields struct {
	// 	KubernetesClient                           kubernetes.Interface
	// 	KubernetesConfigPath                       string
	// 	DryRun                                     bool
	// 	LocalMode                                  bool
	// 	ReapMisconfigured                          bool
	// 	ReapMultiple                               bool
	// 	ReapCrashLoop                              bool
	// 	AllCrashLoop                               bool
	// 	CrashLoopRestartCount                      int
	// 	ReapNotReady                               bool
	// 	ReapNotReadyThreshold                      int
	// 	AllNotReady                                bool
	// 	ReapablePodDisruptionBudgets               []policyv1.PodDisruptionBudget
	// 	ClusterBlockingPodDisruptionBudgets        map[string][]policyv1.PodDisruptionBudget
	// 	NamespacesWithMultiplePodDisruptionBudgets map[string][]policyv1.PodDisruptionBudget
	// 	ExcludedNamespaces                         []string
	// 	ReapablePodDisruptionBudgetsCount          int
	// 	ReapedPodDisruptionBudgetCount             int
	// 	PromPushgateway                            string
	// 	MetricsAPI                                 common.MetricsAPI
	// }
	// type args struct {
	// 	args *Args
	// }
	reaperArgs := &Args{
		LocalMode:             true,
		K8sConfigPath:         "foobar",
		DryRun:                true,
		CrashLoopRestartCount: 1,
	}
	reaperArgs.DryRun = true

	tests := []struct {
		name    string
		fields  ReaperContext
		args    *Args
		wantErr bool
	}{
		// TODO: Add test cases.
		{"Invalid-CrashLoopRestartCount", *_fakeReaperContext(), &Args{
			LocalMode:             false,
			DryRun:                true,
			CrashLoopRestartCount: 1,
		}, false},

		// {"Invalid-CrashLoopRestartCount", ReaperContext{DryRun: true, CrashLoopRestartCount: -1, KubernetesConfigPath: "foobar"}, &Args{
		// 	LocalMode:             true,
		// 	DryRun:                true,
		// 	CrashLoopRestartCount: 1,
		// }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ReaperContext{
				KubernetesClient:                    tt.fields.KubernetesClient,
				KubernetesConfigPath:                tt.fields.KubernetesConfigPath,
				DryRun:                              tt.fields.DryRun,
				LocalMode:                           tt.fields.LocalMode,
				ReapMisconfigured:                   tt.fields.ReapMisconfigured,
				ReapMultiple:                        tt.fields.ReapMultiple,
				ReapCrashLoop:                       tt.fields.ReapCrashLoop,
				AllCrashLoop:                        tt.fields.AllCrashLoop,
				CrashLoopRestartCount:               tt.fields.CrashLoopRestartCount,
				ReapNotReady:                        tt.fields.ReapNotReady,
				ReapNotReadyThreshold:               tt.fields.ReapNotReadyThreshold,
				AllNotReady:                         tt.fields.AllNotReady,
				ReapablePodDisruptionBudgets:        tt.fields.ReapablePodDisruptionBudgets,
				ClusterBlockingPodDisruptionBudgets: tt.fields.ClusterBlockingPodDisruptionBudgets,
				NamespacesWithMultiplePodDisruptionBudgets: tt.fields.NamespacesWithMultiplePodDisruptionBudgets,
				ExcludedNamespaces:                         tt.fields.ExcludedNamespaces,
				ReapablePodDisruptionBudgetsCount:          tt.fields.ReapablePodDisruptionBudgetsCount,
				ReapedPodDisruptionBudgetCount:             tt.fields.ReapedPodDisruptionBudgetCount,
				PromPushgateway:                            tt.fields.PromPushgateway,
				MetricsAPI:                                 tt.fields.MetricsAPI,
			}
			if err := ctx.validate(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("ReaperContext.validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
