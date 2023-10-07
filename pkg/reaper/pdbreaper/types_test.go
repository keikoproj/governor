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
	"testing"

	"github.com/keikoproj/governor/pkg/reaper/common"
)

func TestReaperContext_validate(t *testing.T) {
	reaperArgsValid := Args{
		LocalMode:             true,
		K8sConfigPath:         common.HomeDir() + "/.kube/config",
		DryRun:                true,
		CrashLoopRestartCount: 1,
	}

	reaperArgsInvalidK8sConfigPath := Args(reaperArgsValid)
	reaperArgsInvalidK8sConfigPath.K8sConfigPath = "/tmp/invalid/path"

	tests := []struct {
		name    string
		fields  ReaperContext
		args    *Args
		wantErr bool
	}{
		{"Valid-Args", *_fakeReaperContext(), &reaperArgsValid, false},
		{"Invalid-K8sConfigPath", *_fakeReaperContext(), &reaperArgsInvalidK8sConfigPath, true},
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
