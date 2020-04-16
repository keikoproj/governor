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
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"

	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var log = logrus.New()

const (
	// PodCompletedReason is the reason name for for completed pods
	PodCompletedReason = "Completed"
	// PodFailedReason is the reason name for for failed pods
	PodFailedReason = "Failed"
)

// Run is the main runner function for pod-reaper, and will initialize and start the pod-reaper
func Run(ctx *ReaperContext) error {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	err := ctx.getPods()
	if err != nil {
		return errors.Wrap(err, "failed to list pods")
	}

	ctx.deriveStuckPods()
	ctx.deriveCompletedPods()
	ctx.deriveFailedPods()

	if ctx.isQueueEmpty() {
		log.Info("no reapable pods found")
		return nil
	}

	err = ctx.Reap()
	if err != nil {
		return errors.Wrap(err, "failed to reap pods")
	}

	return nil
}

func (ctx *ReaperContext) Reap() error {
	err := ctx.reapPods(ctx.StuckPods)
	if err != nil {
		return errors.Wrap(err, "failed to reap stuck pods")
	}

	err = ctx.reapPods(ctx.CompletedPods)
	if err != nil {
		return errors.Wrap(err, "failed to reap completed pods")
	}

	err = ctx.reapPods(ctx.FailedPods)
	if err != nil {
		return errors.Wrap(err, "failed to reap failed pods")
	}
	return nil
}

func (ctx *ReaperContext) isQueueEmpty() bool {
	if len(ctx.StuckPods) != 0 {
		return false
	}

	if ctx.ReapCompleted && len(ctx.CompletedPods) != 0 {
		return false
	}

	if ctx.ReapFailed && len(ctx.FailedPods) != 0 {
		return false
	}

	return true
}

func (ctx *ReaperContext) reapPods(pods map[string]string) error {
	corev1 := ctx.KubernetesClient.CoreV1()
	// Iterate stuck pods and reap if not dryRun
	for pod, namespace := range pods {
		log.Infof("reaping %v/%v", namespace, pod)
		gracePeriod := int64(0)
		forceDeleteOpts := &metav1.DeleteOptions{}
		forceDeleteOpts.GracePeriodSeconds = &gracePeriod

		// Dump pod json
		podObject, err := corev1.Pods(namespace).Get(pod, metav1.GetOptions{})
		if err != nil {
			log.Warnf("failed to dump pod spec, %v", err)
		}

		podDump, err := json.Marshal(podObject)
		if err != nil {
			log.Warnf("failed to dump pod spec, %v", err)
		}

		log.Infof("pod dump: %v", string(podDump))

		if !ctx.DryRun {
			err := corev1.Pods(namespace).Delete(pod, forceDeleteOpts)
			if err != nil {
				return err
			}
			ctx.ReapedPods++
			log.Infof("%v/%v has been reaped", namespace, pod)
		} else {
			log.Warnf("dry-run is on, pod will not be reaped")
		}
	}
	return nil
}

func podHasRunningContainers(pod v1.Pod) bool {
	var (
		runningContainers    int
		terminatedContainers int
		containerStatuses    = pod.Status.ContainerStatuses
	)

	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Running != nil {
			runningContainers++
		} else {
			terminatedContainers++
		}
	}
	if runningContainers > 0 {
		log.Infof("%v/%v is not reapable - running containers detected", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		return true
	}

	return false
}

func (ctx *ReaperContext) deriveCompletedPods() {

	if !ctx.ReapCompleted {
		return
	}

	now := time.Now().UTC()
	for _, pod := range ctx.AllPods.Items {
		var (
			containerStatuses = pod.Status.ContainerStatuses
			times             = make([]time.Time, 0)
			podName           = pod.ObjectMeta.Name
			podNamespace      = pod.ObjectMeta.Namespace
		)

		// When softReap mode is On, only pods with 0 running containers are reapable
		if ctx.SoftReap && podHasRunningContainers(pod) {
			log.Infof("%v/%v is not reapable - running containers detected", podNamespace, podName)
			continue
		}

		// If pod phase is not completed, skip
		if pod.Status.Phase != v1.PodSucceeded {
			continue
		}

		for _, containerStatus := range containerStatuses {
			if containerStatus.State.Terminated != nil {
				times = append(times, containerStatus.State.Terminated.FinishedAt.Time)
			}
		}

		sort.Sort(FinishTimes(times))
		lastFinishedContainerTime := times[len(times)-1]
		diff := now.Sub(lastFinishedContainerTime).Minutes()

		// Determine if pod is stuck deleting
		if diff > ctx.ReapCompletedAfter {
			log.Infof("%v/%v is reapable !! all containers completed for diff: %.2f/%v", podNamespace, podName, diff, ctx.ReapCompletedAfter)
			ctx.CompletedPods[podName] = podNamespace
		}
	}
}

func (ctx *ReaperContext) deriveFailedPods() {

	if !ctx.ReapFailed {
		return
	}

	now := time.Now().UTC()
	for _, pod := range ctx.AllPods.Items {
		var (
			containerStatuses = pod.Status.ContainerStatuses
			times             = make([]time.Time, 0)
			podName           = pod.ObjectMeta.Name
			podNamespace      = pod.ObjectMeta.Namespace
		)

		// When softReap mode is On, only pods with 0 running containers are reapable
		if ctx.SoftReap && podHasRunningContainers(pod) {
			log.Infof("%v/%v is not reapable - running containers detected", podNamespace, podName)
			continue
		}

		// If pod phase is not failed, skip
		if pod.Status.Phase != v1.PodFailed {
			continue
		}

		for _, containerStatus := range containerStatuses {
			if containerStatus.State.Terminated != nil {
				times = append(times, containerStatus.State.Terminated.FinishedAt.Time)
			}
		}

		sort.Sort(FinishTimes(times))
		lastFinishedContainerTime := times[len(times)-1]
		diff := now.Sub(lastFinishedContainerTime).Minutes()

		// Determine if pod is stuck deleting
		if diff > ctx.ReapCompletedAfter {
			log.Infof("%v/%v is reapable !! pod in failed state for diff: %.2f/%v", podNamespace, podName, diff, ctx.ReapCompletedAfter)
			ctx.CompletedPods[podName] = podNamespace
		}
	}
}

func (ctx *ReaperContext) deriveStuckPods() {
	now := time.Now().UTC()
	for _, pod := range ctx.TerminatingPods.Items {
		var (
			podName                = pod.ObjectMeta.Name
			podNamespace           = pod.ObjectMeta.Namespace
			deletionGracePeriod    = *pod.ObjectMeta.DeletionGracePeriodSeconds
			terminationGracePeriod = *pod.Spec.TerminationGracePeriodSeconds
			totalGracePeriod       = deletionGracePeriod + terminationGracePeriod
			deletionTimestamp      = pod.ObjectMeta.DeletionTimestamp.Add(time.Duration(-totalGracePeriod) * time.Second).UTC()
			deletionDiff           = now.Sub(deletionTimestamp).Minutes()
		)
		log.Infof("%v/%v total grace period = %vs", podNamespace, podName, totalGracePeriod)
		log.Infof("%v/%v has been terminating since %v, diff: %.2f/%v", podNamespace, podName, deletionTimestamp, deletionDiff, ctx.TimeToReap)

		// When softReap mode is On, only pods with 0 running containers are reapable
		if ctx.SoftReap && podHasRunningContainers(pod) {
			log.Infof("%v/%v is not reapable - running containers detected", podNamespace, podName)
			continue
		}

		// Determine if pod is stuck deleting
		if deletionDiff > ctx.TimeToReap {
			log.Infof("%v/%v is reapable !!", podNamespace, podName)
			ctx.StuckPods[podName] = podNamespace
		}
	}
}

func (ctx *ReaperContext) getPods() error {
	log.Infoln("starting scan cycle")
	terminatingPods := &v1.PodList{}
	corev1 := ctx.KubernetesClient.CoreV1()

	// get pods in all namespaces
	allPods, err := corev1.Pods("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	ctx.AllPods = allPods

	log.Infof("found %v pods", len(allPods.Items))

	// get pods who deletion timestamp is not zero (deletion started)
	for _, pod := range allPods.Items {
		if !pod.ObjectMeta.DeletionTimestamp.IsZero() {
			log.Infof("%v/%v is being deleted", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
			terminatingPods.Items = append(terminatingPods.Items, pod)
		}
	}

	ctx.TerminatingPods = terminatingPods
	return nil
}

func (ctx *ReaperContext) ValidateArguments(args *Args) error {
	ctx.StuckPods = make(map[string]string)
	ctx.CompletedPods = make(map[string]string)
	ctx.FailedPods = make(map[string]string)
	ctx.DryRun = args.DryRun
	ctx.ReapCompleted = args.ReapCompleted
	ctx.ReapFailed = args.ReapFailed
	ctx.ReapCompletedAfter = args.ReapCompletedAfter
	ctx.ReapFailedAfter = args.ReapFailedAfter

	ctx.SoftReap = args.SoftReap
	if !ctx.SoftReap {
		log.Warn("--soft-reap is off, stuck pods with running containers will be reaped")
	}

	if ctx.ReapCompleted && ctx.ReapCompletedAfter < 1 {
		err := fmt.Errorf("--reap-completed-after must be set to a number greater than or equal to 1")
		log.Errorln(err)
		return err
	}

	if ctx.ReapFailed && ctx.ReapFailedAfter < 1 {
		err := fmt.Errorf("--reap-failed-after must be set to a number greater than or equal to 1")
		log.Errorln(err)
		return err
	}

	ctx.TimeToReap = args.ReapAfter
	if ctx.TimeToReap < 1 {
		err := fmt.Errorf("--reap-after must be set to a number greater than or equal to 1")
		log.Errorln(err)
		return err
	}

	if args.K8sConfigPath != "" {
		ok := common.PathExists(args.K8sConfigPath)
		if !ok {
			err := fmt.Errorf("--kubeconfig flag path '%v' does not exist", ctx.KubernetesConfigPath)
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
	} else {
		var err error
		ctx.KubernetesClient, err = common.InClusterAuth()
		if err != nil {
			log.Errorln("in-cluster auth failed")
			return err
		}
	}

	return nil
}
