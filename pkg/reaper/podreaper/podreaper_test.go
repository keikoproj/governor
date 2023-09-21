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
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/keikoproj/governor/pkg/reaper/common"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var loggingEnabled bool

const (
	excludedNamespace          = "excluded-ns"
	stuckExcludedNamespace     = "stuck-excluded-ns"
	completedExcludedNamespace = "completed-excluded-ns"
	failedExcludedNamespace    = "failed-excluded-ns"
	includedNamespace          = "incuded-ns"
)

func init() {
	flag.BoolVar(&loggingEnabled, "logging-enabled", false, "Enable Reaper Logs")
}

func _getReapable(ctx *ReaperContext) int {
	return len(ctx.StuckPods) + len(ctx.CompletedPods) + len(ctx.FailedPods)
}

func newFakeReaperContext() *ReaperContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}
	ctx := ReaperContext{}
	ctx.StuckPods = make(map[string]string)
	ctx.CompletedPods = make(map[string]string)
	ctx.FailedPods = make(map[string]string)
	ctx.KubernetesClient = fake.NewSimpleClientset()
	loadFakeAPI(&ctx)
	// Default Flags
	ctx.TimeToReap = 10
	ctx.DryRun = false
	ctx.SoftReap = true
	ctx.ReapCompleted = true
	ctx.ReapCompletedAfter = 10
	ctx.ReapFailed = true
	ctx.ReapFailedAfter = 10
	return &ctx
}

func getContainerStatus(name, reason string, finishedAt time.Time) v1.ContainerStatus {
	return v1.ContainerStatus{
		Name: name,
		State: v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				Reason:     reason,
				FinishedAt: metav1.Time{Time: finishedAt},
			},
		},
	}
}

func loadFakeAPI(ctx *ReaperContext) {
	fakeNamespaces := []*v1.Namespace{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: includedNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: excludedNamespace,
				Annotations: map[string]string{
					NamespaceExclusionAnnotationKey: NamespaceExclusionEnabledAnnotationValue,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: stuckExcludedNamespace,
				Annotations: map[string]string{
					NamespaceStuckExclusionAnnotationKey: NamespaceExclusionEnabledAnnotationValue,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: completedExcludedNamespace,
				Annotations: map[string]string{
					NamespaceCompletedExclusionAnnotationKey: NamespaceExclusionEnabledAnnotationValue,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: failedExcludedNamespace,
				Annotations: map[string]string{
					NamespaceFailedExclusionAnnotationKey: NamespaceExclusionEnabledAnnotationValue,
				},
			},
		},
	}

	// Create fake namespaces
	for _, ns := range fakeNamespaces {
		ctx.KubernetesClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	}
}

func createFakePods(pods []FakePod, ctx *ReaperContext) {
	for _, c := range pods {

		if c.podName == "" {
			c.podName = fmt.Sprintf("%v-%v", strings.ToLower(randomdata.SillyName()), randomdata.Number(10000, 99999))
		}

		if c.podNamespace == "" {
			c.podNamespace = includedNamespace
		}

		if c.deletionGracePeriod == nil {
			c.deletionGracePeriod = aws.Int64(30)
		}

		if c.terminationGracePeriod == nil {
			c.terminationGracePeriod = aws.Int64(30)
		}

		pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:                       c.podName,
			Namespace:                  c.podNamespace,
			DeletionGracePeriodSeconds: c.deletionGracePeriod,
		},
			Spec: v1.PodSpec{
				TerminationGracePeriodSeconds: c.terminationGracePeriod,
			},
		}

		if c.isTerminating {
			if c.terminatingTime.IsZero() {
				c.terminatingTime = time.Now()
			}
			totalGracePeriod := *c.deletionGracePeriod + *c.terminationGracePeriod
			adjustedNow := c.terminatingTime.Add(time.Duration(totalGracePeriod) * time.Second)
			pod.DeletionTimestamp = &metav1.Time{Time: adjustedNow}
		}

		if c.phase != "" {
			pod.Status.Phase = c.phase
		}

		if len(c.containers) != 0 {
			for _, c := range c.containers {
				pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, c)
			}
		}

		for i := 1; i <= c.runningContainers; i++ {
			runningState := v1.ContainerStateRunning{StartedAt: metav1.Time{Time: time.Now()}}
			runningContainerStatus := v1.ContainerStatus{Name: fmt.Sprintf("container-%v", i), State: v1.ContainerState{Running: &runningState}}
			pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, runningContainerStatus)
		}

		for i := 1; i <= c.terminatedContainers; i++ {
			terminatingState := v1.ContainerStateTerminated{ExitCode: 0}
			runningContainerStatus := v1.ContainerStatus{Name: fmt.Sprintf("container-%v", i), State: v1.ContainerState{Terminated: &terminatingState}}
			pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, runningContainerStatus)
		}

		ctx.KubernetesClient.CoreV1().Pods(c.podNamespace).Create(context.Background(), pod, metav1.CreateOptions{})
	}
}

func (u *ReaperUnitTest) Run(t *testing.T, timeTest bool) {
	createFakePods(u.Pods, u.FakeReaper)
	start := time.Now()
	Run(u.FakeReaper)
	secondsSince := int(time.Since(start).Seconds())

	if timeTest {
		if secondsSince != u.ExpectedDurationSeconds {
			t.Fatalf("expected Duration: %vs, got: %vs", u.ExpectedDurationSeconds, secondsSince)
		}
		return
	}

	if len(u.FakeReaper.TerminatingPods.Items) != u.ExpectedTerminating {
		t.Fatalf("expected TerminatingPods: %v, got: %v", u.ExpectedTerminating, len(u.FakeReaper.TerminatingPods.Items))
	}

	if len(u.FakeReaper.CompletedPods) != u.ExpectedCompleted {
		t.Fatalf("expected CompletedPods: %v, got: %v", u.ExpectedCompleted, len(u.FakeReaper.CompletedPods))
	}

	if len(u.FakeReaper.FailedPods) != u.ExpectedFailed {
		t.Fatalf("expected FailedPods: %v, got: %v", u.ExpectedFailed, len(u.FakeReaper.FailedPods))
	}

	if len(u.FakeReaper.StuckPods) != u.ExpectedStuck {
		t.Fatalf("expected StuckPods: %v, got: %v", u.ExpectedStuck, len(u.FakeReaper.StuckPods))
	}

	reapable := _getReapable(u.FakeReaper)
	if reapable != u.ExpectedReapable {
		t.Fatalf("expected Reapable: %v, got: %v", u.ExpectedReapable, reapable)
	}

	if u.FakeReaper.ReapedPods != u.ExpectedReaped {
		t.Fatalf("expected Reaped: %v, got: %v", u.ExpectedReaped, u.FakeReaper.ReapedPods)
	}

}

type FakePod struct {
	podName                string
	podNamespace           string
	deletionGracePeriod    *int64
	terminationGracePeriod *int64
	isTerminating          bool
	phase                  v1.PodPhase
	terminatingTime        time.Time
	containers             []v1.ContainerStatus
	runningContainers      int
	terminatedContainers   int
}

type ReaperUnitTest struct {
	TestDescription         string
	Pods                    []FakePod
	FakeReaper              *ReaperContext
	ExpectedTerminating     int
	ExpectedStuck           int
	ExpectedCompleted       int
	ExpectedFailed          int
	ExpectedReapable        int
	ExpectedReaped          int
	ExpectedDurationSeconds int
}

func TestDeriveStatePositive(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Derive State - able to detect terminating pod",
		Pods: []FakePod{
			{
				isTerminating: true,
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedReapable:    0,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestDeriveStateNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Derive State - able to detect non-terminating pod",
		Pods: []FakePod{
			{
				isTerminating: false,
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 0,
		ExpectedReapable:    0,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestDeriveTimeReapablePositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.TimeToReap = 5
	testCase := ReaperUnitTest{
		TestDescription: "Derive Reapable - able to detect time reapable pods",
		Pods: []FakePod{
			{
				isTerminating:   true,
				terminatingTime: time.Now().Add(time.Duration(-8) * time.Minute),
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedStuck:       1,
		ExpectedReapable:    1,
		ExpectedReaped:      1,
	}
	testCase.Run(t, false)
}

func TestDeriveTimeReapableNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.TimeToReap = 5
	testCase := ReaperUnitTest{
		TestDescription: "Derive Reapable - able to detect unreapable pods",
		Pods: []FakePod{
			{
				isTerminating:   true,
				terminatingTime: time.Now().Add(time.Duration(-4) * time.Minute),
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedReapable:    0,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestReapCompletedFailedDisable(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Reap Completed/Failed disabled - should honor flag",
		Pods: []FakePod{
			{
				phase: v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-11)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				phase: v1.PodFailed,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodFailedReason, time.Now().Add(time.Duration(-50)*time.Minute)),
					getContainerStatus("container-2", PodFailedReason, time.Now().Add(time.Duration(-80)*time.Minute)),
				},
			},
			{
				phase:      v1.PodFailed,
				containers: []v1.ContainerStatus{},
			},
		},
		FakeReaper:        reaper,
		ExpectedCompleted: 0,
		ExpectedReapable:  0,
		ExpectedReaped:    0,
	}
	testCase.FakeReaper.ReapCompleted = false
	testCase.FakeReaper.ReapFailed = false
	testCase.Run(t, false)
}

func TestReapCompletedPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Reap Completed - able to reap completed pods",
		Pods: []FakePod{
			{
				phase: v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-10)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				phase: v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-5)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-10)*time.Minute)),
				},
			},
		},
		FakeReaper:        reaper,
		ExpectedCompleted: 1,
		ExpectedReapable:  1,
		ExpectedReaped:    1,
	}
	testCase.Run(t, false)
}

func TestReapCompletedNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Reap Completed - should not reap non completed pods",
		Pods: []FakePod{
			{
				phase: v1.PodFailed,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodFailedReason, time.Now().Add(time.Duration(-11)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				phase: v1.PodRunning,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", "running", time.Now().Add(time.Duration(-50)*time.Minute)),
					getContainerStatus("container-2", "running", time.Now().Add(time.Duration(-80)*time.Minute)),
				},
			},
		},
		FakeReaper:        reaper,
		ExpectedCompleted: 0,
		ExpectedFailed:    1,
		ExpectedReapable:  1,
		ExpectedReaped:    1,
	}
	testCase.Run(t, false)
}

func TestReapFailedPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Reap Completed - able to reap failed pods",
		Pods: []FakePod{
			{
				phase: v1.PodFailed,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
					getContainerStatus("container-1", PodFailedReason, time.Now().Add(time.Duration(-10)*time.Minute)),
				},
			},
			{
				phase: v1.PodFailed,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodFailedReason, time.Now().Add(time.Duration(-5)*time.Minute)),
					getContainerStatus("container-2", PodFailedReason, time.Now().Add(time.Duration(-10)*time.Minute)),
				},
			},
			{
				phase:      v1.PodFailed,
				containers: []v1.ContainerStatus{},
			},
		},
		FakeReaper:       reaper,
		ExpectedFailed:   2,
		ExpectedReapable: 2,
		ExpectedReaped:   2,
	}
	testCase.Run(t, false)
}

func TestReapFailedNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Reap Completed - should not reap non failed pods",
		Pods: []FakePod{
			{
				phase: v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-11)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				phase: v1.PodRunning,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", "running", time.Now().Add(time.Duration(-50)*time.Minute)),
					getContainerStatus("container-2", "running", time.Now().Add(time.Duration(-80)*time.Minute)),
				},
			},
		},
		FakeReaper:        reaper,
		ExpectedCompleted: 1,
		ExpectedFailed:    0,
		ExpectedReapable:  1,
		ExpectedReaped:    1,
	}
	testCase.Run(t, false)
}

func TestNamespaceExclusionPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Namespace Exclusion - should exclude namespace with annotation",
		Pods: []FakePod{
			{
				isTerminating:   true,
				podNamespace:    stuckExcludedNamespace,
				terminatingTime: time.Now().Add(time.Duration(-11) * time.Minute),
			},
			{
				podNamespace: excludedNamespace,
				phase:        v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-11)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				podNamespace: completedExcludedNamespace,
				phase:        v1.PodSucceeded,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodCompletedReason, time.Now().Add(time.Duration(-11)*time.Minute)),
					getContainerStatus("container-2", PodCompletedReason, time.Now().Add(time.Duration(-15)*time.Minute)),
				},
			},
			{
				podNamespace: failedExcludedNamespace,
				phase:        v1.PodFailed,
				containers: []v1.ContainerStatus{
					getContainerStatus("container-1", PodFailedReason, time.Now().Add(time.Duration(-50)*time.Minute)),
					getContainerStatus("container-2", PodFailedReason, time.Now().Add(time.Duration(-80)*time.Minute)),
				},
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedStuck:       0,
		ExpectedCompleted:   0,
		ExpectedFailed:      0,
		ExpectedReapable:    0,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestDryRunPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.TimeToReap = 5
	reaper.DryRun = true
	testCase := ReaperUnitTest{
		TestDescription: "Dry Run - pods not reaped when on",
		Pods: []FakePod{
			{
				isTerminating:   true,
				terminatingTime: time.Now().Add(time.Duration(-8) * time.Minute),
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedStuck:       1,
		ExpectedReapable:    1,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestSoftReapPositive(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.TimeToReap = 5
	reaper.SoftReap = true
	testCase := ReaperUnitTest{
		TestDescription: "Soft Reap - will not terminate pods with running containers",
		Pods: []FakePod{
			{
				isTerminating:        true,
				terminatingTime:      time.Now().Add(time.Duration(-8) * time.Minute),
				runningContainers:    1,
				terminatedContainers: 1,
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedReapable:    0,
		ExpectedReaped:      0,
	}
	testCase.Run(t, false)
}

func TestSoftReapNegative(t *testing.T) {
	reaper := newFakeReaperContext()
	reaper.TimeToReap = 5
	reaper.SoftReap = false
	testCase := ReaperUnitTest{
		TestDescription: "Soft Reap - will terminate pods with running containers when off",
		Pods: []FakePod{
			{
				isTerminating:        true,
				terminatingTime:      time.Now().Add(time.Duration(-8) * time.Minute),
				runningContainers:    1,
				terminatedContainers: 1,
			},
		},
		FakeReaper:          reaper,
		ExpectedTerminating: 1,
		ExpectedStuck:       1,
		ExpectedReapable:    1,
		ExpectedReaped:      1,
	}
	testCase.Run(t, false)
}

func TestValidateArgumentsPositive(t *testing.T) {
	reaper := ReaperContext{}
	reaperArgs := &Args{
		ReapAfter: 10,
		LocalMode: true,
		SoftReap:  true,
		DryRun:    true,
	}
	reaper.ValidateArguments(reaperArgs)
	if reaper.TimeToReap != reaperArgs.ReapAfter {
		t.Fatalf("expected TimeToReap: %v, got: %v", reaperArgs.ReapAfter, reaper.TimeToReap)
	}
	if reaper.SoftReap != reaperArgs.SoftReap {
		t.Fatalf("expected SoftReap: %v, got: %v", reaperArgs.SoftReap, reaper.SoftReap)
	}
	if reaper.DryRun != reaperArgs.DryRun {
		t.Fatalf("expected DryRun: %v, got: %v", reaperArgs.DryRun, reaper.DryRun)
	}
}

func TestValidateArgumentsNegative(t *testing.T) {
	reaper := ReaperContext{}
	reaperArgs := &Args{
		ReapAfter: 0,
		LocalMode: true,
		SoftReap:  false,
		DryRun:    false,
	}
	err := reaper.ValidateArguments(reaperArgs)
	if err == nil {
		t.Fatalf("expected Error: %v, got: %v", "--reap-after must be set to a number greater than or equal to 1", err)
	}
	if reaper.TimeToReap != reaperArgs.ReapAfter {
		t.Fatalf("expected TimeToReap: %v, got: %v", reaperArgs.ReapAfter, reaper.TimeToReap)
	}
	if reaper.SoftReap != reaperArgs.SoftReap {
		t.Fatalf("expected SoftReap: %v, got: %v", reaperArgs.SoftReap, reaper.SoftReap)
	}
	if reaper.DryRun != reaperArgs.DryRun {
		t.Fatalf("expected DryRun: %v, got: %v", reaperArgs.DryRun, reaper.DryRun)
	}
}
