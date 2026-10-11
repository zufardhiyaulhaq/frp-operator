package frps

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPodState(t *testing.T) {
	ready := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	if ok, problem := PodState(ready); !ok || problem != "" {
		t.Errorf("ready pod = %v, %q", ok, problem)
	}
	creating := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "frps", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}}}}}
	if ok, problem := PodState(creating); ok || problem != "" {
		t.Errorf("creating pod = %v, %q", ok, problem)
	}
	crashing := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "frps", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 10s"}}}}}}
	if ok, problem := PodState(crashing); ok || !strings.Contains(problem, "CrashLoopBackOff") {
		t.Errorf("crashing pod = %v, %q", ok, problem)
	}
}
