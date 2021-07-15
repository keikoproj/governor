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
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var log = logrus.New()

const (
	ReasonCrashLoopBackOff = "CrashLoopBackOff"

	EventReasonPodDisruptionBudgetDeleted = "PodDisruptionBudgetDeleted"
	EventReasonBlockingDetected           = "BlockingPodDisruptionBudget"
	EventReasonMultipleDetected           = "MultiplePodDisruptionBudgets"
	EventReasonBlockingCrashLoopDetected  = "BlockingPodDisruptionBudgetWithCrashLoop"

	EventMessageDeletedFmt   = "The PodDisruptionBudget %v has been deleted by pdb-reaper due to violation"
	EventMessageBlockingFmt  = "The PodDisruptionBudget %v has been marked for deletion due to misconfiguration/not allowing disruptions"
	EventMessageMultipleFmt  = "The PodDisruptionBudget %v has been marked for deletion due to multiple budgets targeting same pods"
	EventMessageCrashLoopFmt = "The PodDisruptionBudget %v has been marked for deletion due to pods in CrashLoopBackOff blocking disruptions"
)

// Run is the main runner function for pdb-reaper, and will initialize and start the pdb-reaper
func Run(args *Args) error {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	ctx := NewReaperContext(args)

	err := ctx.execute()
	if err != nil {
		return errors.Wrap(err, "execution failed")
	}

	return nil
}

func (ctx *ReaperContext) execute() error {
	log.Info("pdb-reaper starting")

	if err := ctx.scan(); err != nil {
		return errors.Wrap(err, "failed to scan cluster")
	}

	if err := ctx.reap(); err != nil {
		return errors.Wrap(err, "failed to reap PDBs")
	}
	return nil
}

func (ctx *ReaperContext) reap() error {

	err := ctx.handleMultipleDisruptionBudgets()
	if err != nil {
		return errors.Wrap(err, "failed to handle multiple PDBs")
	}

	err = ctx.handleBlockingDisruptionBudgets()
	if err != nil {
		return errors.Wrap(err, "failed to handle blocking PDBs")
	}

	err = ctx.handleReapableDisruptionBudgets()
	if err != nil {
		return errors.Wrap(err, "failed to handle reapable PDBs")
	}

	return nil
}

func (ctx *ReaperContext) scan() error {

	var (
		namespacedPDBs = make(map[string][]policyv1beta1.PodDisruptionBudget)
	)
	pdbs, err := ctx.KubernetesClient.PolicyV1beta1().PodDisruptionBudgets("").List(metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list PDBs")
	}

	for _, pdb := range pdbs.Items {
		namespace := pdb.GetNamespace()
		namespacedPDBs[namespace] = append(namespacedPDBs[namespace], pdb)
	}

	for namespace, pdbs := range namespacedPDBs {
		if len(pdbs) > 1 {
			ctx.NamespacesWithMultiplePodDisruptionBudgets[namespace] = append(ctx.NamespacesWithMultiplePodDisruptionBudgets[namespace], pdbs...)
		}
	}

	for _, pdb := range pdbs.Items {
		var (
			namespace = pdb.GetNamespace()
		)

		// if pdb is allowing disruptions, it is non-blocking
		if pdb.Status.PodDisruptionsAllowed != 0 {
			log.Infof("ignoring pdb %v since it is allowing %v disruptions", pdbNamespacedName(pdb), pdb.Status.PodDisruptionsAllowed)
			continue
		}
		// if no pods match the selector / expected, it is non-blocking
		if pdb.Status.ExpectedPods == 0 {
			log.Infof("ignoring pdb %v since it is expecting 0 pods", pdbNamespacedName(pdb))
			continue
		}

		ctx.ClusterBlockingPodDisruptionBudgets[namespace] = append(ctx.ClusterBlockingPodDisruptionBudgets[namespace], pdb)
	}

	return nil
}

func (ctx *ReaperContext) handleReapableDisruptionBudgets() error {
	for _, pdb := range ctx.ReapablePodDisruptionBudgets {
		var (
			name      = pdb.GetName()
			namespace = pdb.GetNamespace()
		)
		log.Infof("deleting offending PDB %v", pdbNamespacedName(pdb))

		pdbDump, err := json.Marshal(pdb)
		if err != nil {
			return errors.Wrap(err, "failed to marshal PDB spec")
		}
		log.Infof("PDB dump: %v", string(pdbDump))

		if ctx.DryRun {
			log.Warnf("DryRun is on, PDB %v will not be deleted", pdbNamespacedName(pdb))
			continue
		}

		err = ctx.KubernetesClient.PolicyV1beta1().PodDisruptionBudgets(namespace).Delete(name, &metav1.DeleteOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, "failed to delete offending PDB %v", pdbNamespacedName(pdb))
		}
		err = ctx.publishEvent(pdb, EventReasonPodDisruptionBudgetDeleted, EventMessageDeletedFmt)
		if err != nil {
			log.Warnf(err.Error())
		}
		ctx.ReapedPodDisruptionBudgetCount++

	}
	return nil
}

func (ctx *ReaperContext) handleBlockingDisruptionBudgets() error {

	for namespace, pdbs := range ctx.ClusterBlockingPodDisruptionBudgets {

		for _, pdb := range pdbs {
			log.Infof("evaluating blocking PDB %v", pdbNamespacedName(pdb))
			labelSelector, err := common.GetSelectorString(pdb.Spec.Selector)
			if err != nil {
				return errors.Wrapf(err, "failed to get label selector from structured selector %+v", pdb.Spec.Selector)
			}

			pods, err := ctx.listPodsWithSelector(namespace, labelSelector)
			if err != nil {
				return errors.Wrap(err, "failed to list PDB pods")
			}

			if ctx.ReapMisconfigured {
				misconfigured, err := isMisconfigured(pdb, pods)
				if err != nil {
					return errors.Wrap(err, "failed to determine if PDB is misconfigured")
				}

				if misconfigured {
					log.Infof("PDB %v is marked reapable due to blocking configuration", pdbNamespacedName(pdb))
					ctx.addReapablePodDisruptionBudget(pdb)
					err = ctx.publishEvent(pdb, EventReasonBlockingDetected, EventMessageBlockingFmt)
					if err != nil {
						log.Warnf(err.Error())
					}
				}
			}

			if ctx.ReapCrashLoop {
				if crashLoop := isPodsInCrashloop(pods, ctx.CrashLoopRestartCount, ctx.AllCrashLoop); crashLoop {
					log.Infof("PDB %v is marked reapable due to targeted pods in crashloop: %+v", pdbNamespacedName(pdb), podSliceNamespacedNames(pods))
					ctx.addReapablePodDisruptionBudget(pdb)
					err = ctx.publishEvent(pdb, EventReasonBlockingCrashLoopDetected, EventMessageCrashLoopFmt)
					if err != nil {
						log.Warnf(err.Error())
					}
				}
			}
		}
	}
	return nil
}

func (ctx *ReaperContext) handleMultipleDisruptionBudgets() error {

	if !ctx.ReapMultiple {
		return nil
	}

	for namespace, pdbs := range ctx.NamespacesWithMultiplePodDisruptionBudgets {
		namespacePodsWithBudget := make([]corev1.Pod, 0)

		// check if multiple PDBs in a namespace contain refernece to same pods
		for _, pdb := range pdbs {
			log.Infof("evaluating multi-namespace PDB %v", pdbNamespacedName(pdb))

			labelSelector, err := common.GetSelectorString(pdb.Spec.Selector)
			if err != nil {
				return errors.Wrapf(err, "failed to get label selector from structured selector %+v", pdb.Spec.Selector)
			}

			pods, err := ctx.listPodsWithSelector(namespace, labelSelector)
			if err != nil {
				return errors.Wrap(err, "failed to list PDB pods")
			}

			namespacePodsWithBudget = append(namespacePodsWithBudget, pods...)
		}

		if isContainDuplicatePods(namespacePodsWithBudget) {
			log.Infof("PDBs %+v are marked reapable - pods %+v has multiple PDBs", pdbSliceNamespacedNames(pdbs), podSliceNamespacedNames(namespacePodsWithBudget))
			ctx.addReapablePodDisruptionBudget(pdbs...)
			for _, pdb := range pdbs {
				err := ctx.publishEvent(pdb, EventReasonMultipleDetected, EventMessageMultipleFmt)
				if err != nil {
					log.Warnf(err.Error())
				}
			}
		}
	}
	return nil
}

func (ctx *ReaperContext) listPodsWithSelector(namespace, selector string) ([]corev1.Pod, error) {
	var pods []corev1.Pod
	podList, err := ctx.KubernetesClient.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return pods, errors.Wrapf(err, "failed to list pods with selector '%v'", selector)
	}
	pods = append(pods, podList.Items...)
	return pods, nil
}

func (ctx *ReaperContext) publishEvent(pdb policyv1beta1.PodDisruptionBudget, reason, msg string) error {
	var (
		pdbNamespace   = pdb.GetNamespace()
		pdbName        = pdb.GetName()
		namespacedName = pdbNamespacedName(pdb)
	)

	now := time.Now()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("pdb-reaper-%v", pdbName),
			Namespace:    pdbNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:            "PodDisruptionBudget",
			Namespace:       pdbNamespace,
			Name:            pdbName,
			APIVersion:      pdb.APIVersion,
			UID:             pdb.UID,
			ResourceVersion: pdb.ResourceVersion,
		},
		Reason:         reason,
		Message:        fmt.Sprintf(msg, namespacedName),
		Type:           "Normal",
		FirstTimestamp: metav1.NewTime(now),
		LastTimestamp:  metav1.NewTime(now),
	}
	_, err := ctx.KubernetesClient.CoreV1().Events(pdbNamespace).Create(event)
	if err != nil {
		return errors.Wrap(err, "failed to publish event")
	}
	return nil
}

func (ctx *ReaperContext) addReapablePodDisruptionBudget(pdb ...policyv1beta1.PodDisruptionBudget) {
	for _, p := range ctx.ReapablePodDisruptionBudgets {
		if reflect.DeepEqual(p, pdb) {
			return
		}
	}

	ctx.ReapablePodDisruptionBudgetsCount += len(pdb)
	ctx.ReapablePodDisruptionBudgets = append(ctx.ReapablePodDisruptionBudgets, pdb...)
}

func isMisconfigured(pdb policyv1beta1.PodDisruptionBudget, pods []corev1.Pod) (bool, error) {
	var (
		maxUnavailable = pdb.Spec.MaxUnavailable
		minAvailable   = pdb.Spec.MinAvailable
		podCount       = len(pods)
	)

	switch {
	case maxUnavailable != nil:
		allowedUnavailable, err := intstr.GetValueFromIntOrPercent(maxUnavailable, podCount, true)
		if err != nil {
			return false, errors.Wrapf(err, "failed to get IntStr value from PDB Spec.MaxUnavailable '%+v'", maxUnavailable)
		}

		// if pdb is not allowing any disruptions, it is considered misconfigured
		if allowedUnavailable == 0 {
			log.Infof("pdb %v is misconfigured because allowed unavailable replicas is 0", pdbNamespacedName(pdb))
			return true, nil
		}
	case minAvailable != nil:
		requiredAvailable, err := intstr.GetValueFromIntOrPercent(minAvailable, podCount, true)
		if err != nil {
			return false, errors.Wrapf(err, "failed to get IntStr value from PDB Spec.MinAvailable '%+v'", minAvailable)
		}

		// if pdb is requiring expected pods, it is considered misconfigured
		if requiredAvailable == int(pdb.Status.ExpectedPods) {
			log.Infof("pdb %v is misconfigured because required available replicas matches expected pods", pdbNamespacedName(pdb))
			return true, nil
		}
	default:
		return false, nil
	}

	return false, nil
}

func isContainDuplicatePods(pods []corev1.Pod) bool {
	m := make(map[string]bool)
	for _, pod := range pods {
		name := pod.GetName()
		if _, ok := m[name]; ok {
			return true
		} else {
			m[name] = true
		}
	}
	return false
}

func isPodsInCrashloop(pods []corev1.Pod, threshold int, allPods bool) bool {
	podCount := len(pods)
	var crashingCount int
	for _, pod := range pods {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil && containerStatus.RestartCount >= int32(threshold) {
				if containerStatus.State.Waiting.Reason == ReasonCrashLoopBackOff {
					crashingCount++
					break
				}
			}
		}
	}
	if !allPods {
		if crashingCount > 0 {
			return true
		}
	} else {
		if crashingCount == podCount {
			return true
		}
	}
	return false
}
