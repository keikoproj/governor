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
	"fmt"

	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/client-go/kubernetes"
)

// Args is the argument struct for pdb-reaper
type Args struct {
	K8sConfigPath     string
	DryRun            bool
	LocalMode         bool
	ReapMisconfigured bool
	ReapMultiple      bool
	ReapCrashLoop     bool
}

// ReaperContext holds the context of the pdb-reaper and target cluster
type ReaperContext struct {
	KubernetesClient                           kubernetes.Interface
	KubernetesConfigPath                       string
	DryRun                                     bool
	LocalMode                                  bool
	ReapMisconfigured                          bool
	ReapMultiple                               bool
	ReapCrashLoop                              bool
	ReapablePodDisruptionBudgets               []policyv1beta1.PodDisruptionBudget
	ClusterBlockingPodDisruptionBudgets        map[string][]policyv1beta1.PodDisruptionBudget
	NamespacesWithMultiplePodDisruptionBudgets map[string][]policyv1beta1.PodDisruptionBudget
	ReapablePodDisruptionBudgetsCount          int
	ReapedPodDisruptionBudgetCount             int
}

func NewReaperContext(args *Args) *ReaperContext {
	ctx := &ReaperContext{
		ReapablePodDisruptionBudgets:               make([]policyv1beta1.PodDisruptionBudget, 0),
		ClusterBlockingPodDisruptionBudgets:        make(map[string][]policyv1beta1.PodDisruptionBudget),
		NamespacesWithMultiplePodDisruptionBudgets: make(map[string][]policyv1beta1.PodDisruptionBudget),
	}

	if err := ctx.validate(args); err != nil {
		log.Fatalf("failed to validate arguments: %v", err.Error())
	}

	return ctx
}

func (ctx *ReaperContext) validate(args *Args) error {
	ctx.DryRun = args.DryRun
	ctx.LocalMode = args.LocalMode
	ctx.ReapMisconfigured = args.ReapMisconfigured
	ctx.ReapCrashLoop = args.ReapCrashLoop
	ctx.ReapMultiple = args.ReapMultiple

	log.Infof("Dry Run = %t", ctx.DryRun)
	log.Infof("Reap Misconfigured PDBs = %t", ctx.ReapMisconfigured)
	log.Infof("Reap PDBs blocked by CrashLoopBackoff = %v", ctx.ReapCrashLoop)
	log.Infof("Reap Multiple PDBs targeting same deployment = %t", ctx.ReapMultiple)

	if args.K8sConfigPath != "" {
		if ok := common.PathExists(args.K8sConfigPath); !ok {
			return errors.Errorf("--kubeconfig path '%v' was not found", ctx.KubernetesConfigPath)
		}
		ctx.KubernetesConfigPath = args.K8sConfigPath
	}

	if args.LocalMode {
		if ctx.KubernetesConfigPath == "" {
			return errors.Errorf("cannot use --local-mode without --kubeconfig")
		}

		var err error
		ctx.KubernetesClient, err = common.OutOfClusterAuth(ctx.KubernetesConfigPath)
		if err != nil {
			return errors.Wrap(err, "cluster external auth failed")
		}

	} else {
		var err error
		ctx.KubernetesClient, err = common.InClusterAuth()
		if err != nil {
			return errors.Wrap(err, "in-cluster auth failed")
		}
	}

	return nil
}

func pdbNamespacedName(pdb policyv1beta1.PodDisruptionBudget) string {
	var (
		name      = pdb.GetName()
		namespace = pdb.GetNamespace()
	)

	return fmt.Sprintf("%v/%v", namespace, name)
}

func pdbSliceNamespacedNames(pdbs []policyv1beta1.PodDisruptionBudget) []string {
	names := make([]string, 0)
	for _, pdb := range pdbs {
		namespacedName := pdbNamespacedName(pdb)
		names = append(names, namespacedName)
	}
	return names
}

func podSliceNamespacedNames(pods []corev1.Pod) []string {
	names := make([]string, 0)
	for _, pod := range pods {
		var (
			name      = pod.GetName()
			namespace = pod.GetNamespace()
		)
		names = append(names, fmt.Sprintf("%v/%v", namespace, name))
	}
	return names
}
