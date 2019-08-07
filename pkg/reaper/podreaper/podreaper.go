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
	"time"

	"github.com/orkaproj/governor/pkg/reaper/common"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var log = logrus.New()

// Run is the main runner function for pod-reaper, and will initialize and start the pod-reaper
func Run(args *Args) error {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	ctx := &ReaperContext{}

	err := ctx.validateArguments(args)
	if err != nil {
		log.Errorf("failed to validate commandline arguments, %v", err)
		return err
	}

	err = ctx.getTerminatingPods()
	if err != nil {
		log.Errorf("failed to get terminating pods, %v", err)
		return err
	}

	if len(ctx.SeenPods.Items) == 0 {
		log.Info("no terminating pods found")
		return nil
	}

	ctx.deriveStuckPods()
	if len(ctx.StuckPods) == 0 {
		log.Info("no reapable pods found")
		return nil
	}

	err = ctx.reapStuckPods()
	if err != nil {
		log.Errorf("failed to reap stuck pods, %v", err)
		return err
	}
	return nil
}

func (ctx *ReaperContext) reapStuckPods() error {
	log.Infoln("start reap cycle")
	corev1 := ctx.KubernetesClient.CoreV1()
	// Iterate stuck pods and reap if not dryRun
	for pod, namespace := range ctx.StuckPods {
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

func (ctx *ReaperContext) deriveStuckPods() {
	now := time.Now().UTC()
	log.Infof("reap time = %v", now)
	log.Infof("reap target threshold = %vm", ctx.TimeToReap)
	// Iterate pods in terminating and determine how many minutes passed since
	for _, pod := range ctx.SeenPods.Items {
		podName := pod.ObjectMeta.Name
		podNamespace := pod.ObjectMeta.Namespace
		containerStatuses := pod.Status.ContainerStatuses
		deletionGracePeriod := *pod.ObjectMeta.DeletionGracePeriodSeconds
		terminationGracePeriod := *pod.Spec.TerminationGracePeriodSeconds
		totalGracePeriod := deletionGracePeriod + terminationGracePeriod
		log.Infof("%v/%v total grace period = %vs", podNamespace, podName, totalGracePeriod)
		// Subtract totalGracePeriod duration from DeletionTimestamp to get the real deletion timestamp
		deletionTimestamp := pod.ObjectMeta.DeletionTimestamp.Add(time.Duration(-totalGracePeriod) * time.Second).UTC()
		// Get the diff between deletion start and now in minutes
		minuteDiff := now.Sub(deletionTimestamp).Minutes()
		log.Infof("%v/%v has been terminating since %v, diff: %.2f/%v", podNamespace, podName, deletionTimestamp, minuteDiff, ctx.TimeToReap)
		// When softReap mode is On, only pods with 0 running containers are reapable
		if ctx.SoftReap {
			var runningContainers int
			var terminatedContainers int
			for _, containerStatus := range containerStatuses {
				if containerStatus.State.Running != nil {
					runningContainers++
				} else {
					terminatedContainers++
				}
			}
			if runningContainers > 0 {
				log.Infof("%v/%v is not reapable - running containers detected", podNamespace, podName)
				continue
			}
		}

		// Delta between deletion start time and now in minutes must be greater than configured reapAfter to become reapable
		if minuteDiff > ctx.TimeToReap {
			log.Infof("%v/%v is reapable !!", podNamespace, podName)
			ctx.StuckPods[podName] = podNamespace
		} else {
			log.Infof("%v/%v is not reapable - did not meet --reap-after threshold", podNamespace, podName)
		}
	}
}

func (ctx *ReaperContext) getTerminatingPods() error {
	log.Infoln("starting scan cycle")
	terminatingPods := &v1.PodList{}
	corev1 := ctx.KubernetesClient.CoreV1()

	// get pods in all namespaces
	allPods, err := corev1.Pods("").List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	log.Infof("found %v pods", len(allPods.Items))

	// get pods who deletion timestamp is not zero (deletion started)
	for _, pod := range allPods.Items {
		if !pod.ObjectMeta.DeletionTimestamp.IsZero() {
			log.Infof("%v/%v is being deleted", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
			terminatingPods.Items = append(terminatingPods.Items, pod)
		}
	}

	ctx.SeenPods = terminatingPods
	return nil
}

func (ctx *ReaperContext) validateArguments(args *Args) error {
	ctx.StuckPods = make(map[string]string)
	ctx.DryRun = args.DryRun

	ctx.SoftReap = args.SoftReap
	if !ctx.SoftReap {
		log.Warn("--soft-reap is off, stuck pods with running containers will be reaped")
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
