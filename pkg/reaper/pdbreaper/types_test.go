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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReaperContext_validate(t *testing.T) {
	reaperArgsValid := Args{
		LocalMode:             false,
		DryRun:                true,
		CrashLoopRestartCount: 1,
		ReapNotReadyThreshold: 1,
		PromPushgateway:       "http://localhost:9091",
		ExcludedNamespaces:    []string{"kube-system"},
	}

	reaperArgsInvalidCrashLoopRestartCount := Args(reaperArgsValid)
	reaperArgsInvalidCrashLoopRestartCount.CrashLoopRestartCount = -99

	reaperArgsInvalidReapNotReadyThreshold := Args(reaperArgsValid)
	reaperArgsInvalidReapNotReadyThreshold.ReapNotReadyThreshold = -99

	reaperArgsInvalidInClusterAuth := Args(reaperArgsValid)
	reaperArgsInvalidInClusterAuth.LocalMode = false

	reaperArgsInvalidLocalMode := Args(reaperArgsValid)
	reaperArgsInvalidLocalMode.LocalMode = true

	reaperArgsInvalidK8sConfigPath := Args(reaperArgsValid)
	reaperArgsInvalidK8sConfigPath.LocalMode = true
	reaperArgsInvalidK8sConfigPath.K8sConfigPath = "/tmp/invalid/path"

	reaperArgsInvalidK8sConfigPath2 := Args(reaperArgsValid)
	reaperArgsInvalidK8sConfigPath2.LocalMode = true
	reaperArgsInvalidK8sConfigPath2.K8sConfigPath = os.Getenv("HOME")

	tests := []struct {
		name       string
		fields     ReaperContext
		args       *Args
		wantErr    bool
		wantErrMsg string
	}{
		// {"Valid-Args", *_fakeReaperContext(), &reaperArgsValid, false},
		{"Invalid-CrashLoopRestartCount", *_fakeReaperContext(), &reaperArgsInvalidCrashLoopRestartCount, true, "--crashloop-restart-count value cannot be less than 1"},
		{"Invalid-ReapNotReadyThreshold", *_fakeReaperContext(), &reaperArgsInvalidReapNotReadyThreshold, true, "--not-ready-threshold-seconds value cannot be less than 1"},
		{"Invalid-InClusterAuth", *_fakeReaperContext(), &reaperArgsInvalidInClusterAuth, true, "in-cluster auth failed: unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined"},
		{"Invalid-LocalMode", *_fakeReaperContext(), &reaperArgsInvalidLocalMode, true, "cannot use --local-mode without --kubeconfig"},
		{"Invalid-K8sConfigPath", *_fakeReaperContext(), &reaperArgsInvalidK8sConfigPath, true, "--kubeconfig path '/tmp/invalid/path' was not found"},
		{"Invalid-K8sConfigPath2", *_fakeReaperContext(), &reaperArgsInvalidK8sConfigPath2, true, "cluster external auth failed: error loading config file"},
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
			} else {
				assert.ErrorContainsf(t, err, tt.wantErrMsg, "expected error containing %q, got %s", tt.wantErrMsg, err.Error())
			}
		})
	}
}
