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
	"flag"
	"io/ioutil"
	"testing"
	"time"

	"github.com/keikoproj/governor/pkg/reaper/common"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	loggingEnabled       bool
	intStrZeroInt        = intstr.FromInt(0)
	intStrOneInt         = intstr.FromInt(1)
	intStrZeroPercent    = intstr.FromString("0%")
	intStrHundredPercent = intstr.FromString("100%")
)

func init() {
	flag.BoolVar(&loggingEnabled, "logging-enabled", false, "Enable Reaper Logs")
}

func _fakeReaperContext() *ReaperContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}
	ctx := &ReaperContext{
		ReapMisconfigured:                          true,
		ReapMultiple:                               true,
		ReapCrashLoop:                              true,
		CrashLoopRestartCount:                      5,
		AllCrashLoop:                               false,
		ReapNotReady:                               true,
		ReapablePodDisruptionBudgets:               make([]policyv1beta1.PodDisruptionBudget, 0),
		ClusterBlockingPodDisruptionBudgets:        make(map[string][]policyv1beta1.PodDisruptionBudget),
		NamespacesWithMultiplePodDisruptionBudgets: make(map[string][]policyv1beta1.PodDisruptionBudget),
		KubernetesClient:                           fake.NewSimpleClientset(),
	}
	return ctx
}

func _selector(s string) *metav1.LabelSelector {
	selector, _ := metav1.ParseToLabelSelector(s)
	return selector
}

func _fakeAPI(u *ReaperUnitTest) {

	for _, n := range u.Mocks.Namespaces {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: n.Name,
		}}
		_, err := u.FakeReaper.KubernetesClient.CoreV1().Namespaces().Create(namespace)
		if err != nil {
			panic(err)
		}
	}

	for _, p := range u.Mocks.Pods {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.Name,
				Namespace: p.Namespace,
				Labels:    p.Labels,
			},
		}
		if p.IsInCrashloop {
			pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: ReasonCrashLoopBackOff,
					},
				},
				RestartCount: p.RestartCount,
			})
		}
		if p.IsNotReady {
			pod.Status.Phase = corev1.PodPending
			pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
				Type:               corev1.ContainersReady,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.Time{Time: time.Now().Add(time.Duration(-50) * time.Second)},
			})
		}

		pod.Status.StartTime = &metav1.Time{Time: time.Now().Add(time.Duration(-100) * time.Second)}
		_, err := u.FakeReaper.KubernetesClient.CoreV1().Pods(p.Namespace).Create(pod)
		if err != nil {
			panic(err)
		}
	}

	for _, p := range u.Mocks.PDBs {
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.Name,
				Namespace: p.Namespace,
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MinAvailable:   p.MinAvailable,
				MaxUnavailable: p.MaxUnavailable,
				Selector:       p.Selector,
			},
			Status: policyv1beta1.PodDisruptionBudgetStatus{
				PodDisruptionsAllowed: p.PodDisruptionsAllowed,
				ExpectedPods:          p.ExpectedPods,
			},
		}
		_, err := u.FakeReaper.KubernetesClient.PolicyV1beta1().PodDisruptionBudgets(p.Namespace).Create(pdb)
		if err != nil {
			panic(err)
		}
	}
}

func (u *ReaperUnitTest) Run(t *testing.T) {

	_fakeAPI(u)

	err := u.FakeReaper.execute()
	if err != nil {
		t.Fatalf("execution failed: %v", err.Error())
	}

	if u.ExpectedReapableBudgets != u.FakeReaper.ReapablePodDisruptionBudgetsCount {
		t.Fatalf("assertion failed, expected reapable: %v, got: %v", u.ExpectedReapableBudgets, u.FakeReaper.ReapablePodDisruptionBudgetsCount)
	}

	if u.ExpectedReapedBudgets != u.FakeReaper.ReapedPodDisruptionBudgetCount {
		t.Fatalf("assertion failed, expected reaped: %v, got: %v", u.ExpectedReapedBudgets, u.FakeReaper.ReapedPodDisruptionBudgetCount)
	}
}

type ReaperUnitTest struct {
	TestDescription         string
	FakeReaper              *ReaperContext
	Mocks                   KubernetesMockAPI
	PodDisruptionBudgets    []policyv1beta1.PodDisruptionBudget
	Pods                    []corev1.Pod
	ExpectedReapableBudgets int
	ExpectedReapedBudgets   int
}

type KubernetesMockAPI struct {
	Namespaces []MockNamespace
	PDBs       []MockPDB
	Pods       []MockPod
}

type MockNamespace struct {
	Name string
}

func _mockNamespace(name string) MockNamespace {
	return MockNamespace{Name: name}
}

type MockPDB struct {
	Name                  string
	Namespace             string
	MinAvailable          *intstr.IntOrString
	MaxUnavailable        *intstr.IntOrString
	Selector              *metav1.LabelSelector
	ExpectedPods          int32
	PodDisruptionsAllowed int32
}

func _mockPDB(name, namespace string, minAvailable, maxUnavailable *intstr.IntOrString, selector *metav1.LabelSelector, expected, disruptions int32) MockPDB {
	return MockPDB{
		Name:                  name,
		Namespace:             namespace,
		MinAvailable:          minAvailable,
		MaxUnavailable:        maxUnavailable,
		Selector:              selector,
		ExpectedPods:          expected,
		PodDisruptionsAllowed: disruptions,
	}
}

type MockPod struct {
	Name          string
	Namespace     string
	Labels        map[string]string
	IsInCrashloop bool
	RestartCount  int32
	IsNotReady    bool
}

func _mockPod(name, namespace string, labels map[string]string, crashloop bool, restarts int32, notReadyState bool) MockPod {
	return MockPod{
		Name:          name,
		Namespace:     namespace,
		Labels:        labels,
		IsInCrashloop: crashloop,
		RestartCount:  restarts,
		IsNotReady:    notReadyState,
	}
}

func TestBasicExecution(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests a basic execution of pdb reaper with an empty API",
		FakeReaper:      reaper,
	}
	testCase.Run(t)
}

func TestMisconfiguredMaxUnavailable(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with misconfigured blocking PDBs (maxUnavailable)",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrZeroInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrZeroPercent, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 2,
		ExpectedReapedBudgets:   2,
	}
	testCase.Run(t)
}

func TestMisconfiguredMinAvailable(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with misconfigured blocking PDBs (minAvailable)",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", &intStrOneInt, nil, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", &intStrHundredPercent, nil, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", &intStrZeroInt, nil, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 2,
		ExpectedReapedBudgets:   2,
	}
	testCase.Run(t)
}

func TestCrashloop(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to crashloop",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, true, 6, false),
				_mockPod("pod-2", "namespace-2", map[string]string{"app": "app-2"}, true, 1, false),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 1,
		ExpectedReapedBudgets:   1,
	}
	testCase.Run(t)
}

func TestAllCrashloop(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.AllCrashLoop = true
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to crashloop",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1a", "namespace-1", map[string]string{"app": "app-1"}, true, 6, false),
				_mockPod("pod-1b", "namespace-1", map[string]string{"app": "app-1"}, true, 6, false),
				_mockPod("pod-1c", "namespace-1", map[string]string{"app": "app-1"}, true, 6, false),
				_mockPod("pod-2a", "namespace-2", map[string]string{"app": "app-2"}, true, 6, false),
				_mockPod("pod-2b", "namespace-2", map[string]string{"app": "app-2"}, false, 6, false),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 1,
		ExpectedReapedBudgets:   1,
	}
	testCase.Run(t)
}

func TestMultiplePDBs(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with multiple PDBs",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 1),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 2,
		ExpectedReapedBudgets:   2,
	}
	testCase.Run(t)
}

func TestDryRun(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.DryRun = true
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with DryRun on",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrZeroInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, true, 5, false),
			},
		},
		ExpectedReapableBudgets: 4,
		ExpectedReapedBudgets:   0,
	}
	testCase.Run(t)
}

func TestAllConditions(t *testing.T) {
	reaper := _fakeReaperContext()
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with all conditions",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
				_mockNamespace("namespace-4"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrZeroInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 0),
				_mockPDB("pdb-4", "namespace-4", nil, &intStrOneInt, _selector("app=app-4"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, true, 5, false),
				_mockPod("pod-5", "namespace-4", map[string]string{"app": "app-4"}, false, 0, true),
			},
		},
		ExpectedReapableBudgets: 5,
		ExpectedReapedBudgets:   5,
	}
	testCase.Run(t)
}

func TestExcludedNamespaces(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.ExcludedNamespaces = []string{"namespace-2", "namespace-3"}
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with all conditions with excluded namespaces",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrZeroInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, true, 5, false),
			},
		},
		ExpectedReapableBudgets: 2,
		ExpectedReapedBudgets:   2,
	}
	testCase.Run(t)
}

func TestFlagConditions(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.ReapCrashLoop = false
	reaper.ReapMisconfigured = false
	reaper.ReapMultiple = false
	testCase := ReaperUnitTest{
		TestDescription: "Tests main flag conditions for reaper",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrZeroInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, true, 5, false),
			},
		},
		ExpectedReapableBudgets: 0,
		ExpectedReapedBudgets:   0,
	}
	testCase.Run(t)
}

func TestNotReadyThresholdMet(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.ReapNotReadyThreshold = 10
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to not-ready state",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 1, true),
				_mockPod("pod-2", "namespace-2", map[string]string{"app": "app-2"}, false, 0, true),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 2,
		ExpectedReapedBudgets:   2,
	}
	testCase.Run(t)
}

func TestNotReadyThresholdNotMet(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.ReapNotReadyThreshold = 100
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to not-ready state",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 1, true),
				_mockPod("pod-2", "namespace-2", map[string]string{"app": "app-2"}, false, 0, true),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 0,
		ExpectedReapedBudgets:   0,
	}
	testCase.Run(t)
}

func TestAllNotReadyThresholdMet(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.AllNotReady = true
	reaper.ReapNotReadyThreshold = 10
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to crashloop",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
				_mockPDB("pdb-2", "namespace-2", nil, &intStrOneInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-3", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 1),
			},
			Pods: []MockPod{
				_mockPod("pod-1a", "namespace-1", map[string]string{"app": "app-1"}, false, 1, true),
				_mockPod("pod-1b", "namespace-1", map[string]string{"app": "app-1"}, false, 0, true),
				_mockPod("pod-1c", "namespace-1", map[string]string{"app": "app-1"}, false, 0, true),
				_mockPod("pod-2a", "namespace-2", map[string]string{"app": "app-2"}, false, 6, true),
				_mockPod("pod-2b", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-3", "namespace-3", map[string]string{"app": "app-3"}, false, 0, false),
			},
		},
		ExpectedReapableBudgets: 1,
		ExpectedReapedBudgets:   1,
	}
	testCase.Run(t)
}

func TestAllNotReadyThresholNotMet(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.AllNotReady = true
	reaper.ReapNotReadyThreshold = 200
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with blocking PDBs due to crashloop",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-5"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-5", nil, &intStrOneInt, _selector("app=app-1"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1a", "namespace-5", map[string]string{"app": "app-1"}, false, 1, true),
				_mockPod("pod-1b", "namespace-5", map[string]string{"app": "app-1"}, false, 0, true),
				_mockPod("pod-1c", "namespace-5", map[string]string{"app": "app-1"}, false, 0, true),
			},
		},
		ExpectedReapableBudgets: 0,
		ExpectedReapedBudgets:   0,
	}
	testCase.Run(t)
}

func TestWithPushgateway(t *testing.T) {
	reaper := _fakeReaperContext()
	reaper.DryRun = true
	reaper.PromPushgateway = "http://127.0.0.1:9091"
	testCase := ReaperUnitTest{
		TestDescription: "Tests execution scenario of pdb reaper with DryRun on",
		FakeReaper:      reaper,
		Mocks: KubernetesMockAPI{
			Namespaces: []MockNamespace{
				_mockNamespace("namespace-1"),
				_mockNamespace("namespace-2"),
				_mockNamespace("namespace-3"),
			},
			PDBs: []MockPDB{
				_mockPDB("pdb-1", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-2", "namespace-1", nil, &intStrOneInt, _selector("app=app-1"), 1, 1),
				_mockPDB("pdb-3", "namespace-2", nil, &intStrZeroInt, _selector("app=app-2"), 1, 0),
				_mockPDB("pdb-4", "namespace-3", nil, &intStrOneInt, _selector("app=app-3"), 1, 0),
			},
			Pods: []MockPod{
				_mockPod("pod-1", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-2", "namespace-1", map[string]string{"app": "app-1"}, false, 0, false),
				_mockPod("pod-3", "namespace-2", map[string]string{"app": "app-2"}, false, 0, false),
				_mockPod("pod-4", "namespace-3", map[string]string{"app": "app-3"}, true, 5, false),
			},
		},
		ExpectedReapableBudgets: 4,
		ExpectedReapedBudgets:   0,
	}
	testCase.Run(t)
}