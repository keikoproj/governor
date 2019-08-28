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
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/keikoproj/governor/pkg/reaper/common"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var loggingEnabled bool

func init() {
	flag.BoolVar(&loggingEnabled, "logging-enabled", false, "Enable Reaper Logs")
}

func newFakeReaperContext() *ReaperContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}
	ctx := ReaperContext{}
	ctx.StuckPods = make(map[string]string)
	ctx.KubernetesClient = fake.NewSimpleClientset()
	loadFakeAPI(&ctx)
	// Default Flags
	ctx.TimeToReap = 10
	ctx.DryRun = false
	ctx.SoftReap = true
	return &ctx
}

func loadFakeAPI(ctx *ReaperContext) {
	fakeNamespaces := []struct {
		namespaceName string
	}{
		{
			namespaceName: "namespace-1",
		},
		{
			namespaceName: "namespace-2",
		},
		{
			namespaceName: "namespace-3",
		},
	}

	// Create fake namespaces
	for _, c := range fakeNamespaces {
		namespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: c.namespaceName,
		}}
		ctx.KubernetesClient.CoreV1().Namespaces().Create(namespace)
	}
}

func createFakePods(pods []FakePod, ctx *ReaperContext) {
	for _, c := range pods {

		if c.podName == "" {
			c.podName = fmt.Sprintf("%v-%v", strings.ToLower(randomdata.SillyName()), randomdata.Number(10000, 99999))
		}

		if c.podNamespace == "" {
			c.podNamespace = "default"
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

		ctx.KubernetesClient.CoreV1().Pods(c.podNamespace).Create(pod)
	}
}

func (u *ReaperUnitTest) Run(t *testing.T, timeTest bool) {
	createFakePods(u.Pods, u.FakeReaper)
	start := time.Now()
	u.FakeReaper.getTerminatingPods()
	u.FakeReaper.deriveStuckPods()
	u.FakeReaper.reapStuckPods()
	secondsSince := int(time.Since(start).Seconds())

	if timeTest {
		if secondsSince != u.ExpectedDurationSeconds {
			t.Fatalf("expected Duration: %vs, got: %vs", u.ExpectedDurationSeconds, secondsSince)
		}
		return
	}

	if len(u.FakeReaper.SeenPods.Items) != u.ExpectedSeen {
		t.Fatalf("expected SeenPods: %v, got: %v", u.ExpectedSeen, len(u.FakeReaper.SeenPods.Items))
	}

	if len(u.FakeReaper.StuckPods) != u.ExpectedReapable {
		t.Fatalf("expected StuckPods: %v, got: %v", u.ExpectedReapable, len(u.FakeReaper.StuckPods))
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
	terminatingTime        time.Time
	runningContainers      int
	terminatedContainers   int
}

type ReaperUnitTest struct {
	TestDescription         string
	Pods                    []FakePod
	FakeReaper              *ReaperContext
	ExpectedSeen            int
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 0,
		ExpectedReaped:   0,
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
		FakeReaper:       reaper,
		ExpectedSeen:     0,
		ExpectedReapable: 0,
		ExpectedReaped:   0,
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 1,
		ExpectedReaped:   1,
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 0,
		ExpectedReaped:   0,
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 1,
		ExpectedReaped:   0,
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 0,
		ExpectedReaped:   0,
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
		FakeReaper:       reaper,
		ExpectedSeen:     1,
		ExpectedReapable: 1,
		ExpectedReaped:   1,
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
	reaper.validateArguments(reaperArgs)
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
	err := reaper.validateArguments(reaperArgs)
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
