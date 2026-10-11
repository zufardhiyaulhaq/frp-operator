package e2e

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pod(status corev1.PodStatus) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "backends", Name: "echo-abc"}, Status: status}
}

func TestPodReady(t *testing.T) {
	ready := pod(corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}})
	notReady := pod(corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}})
	if !podReady(ready) || podReady(notReady) || podReady(pod(corev1.PodStatus{})) {
		t.Fatal("podReady must be true only for a PodReady=True condition")
	}
}

func TestPodProblem(t *testing.T) {
	tests := []struct {
		name   string
		status corev1.PodStatus
		want   []string
	}{
		{
			name: "image pull failure names the container and reason",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "tcp",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "toomanyrequests: rate limit"}},
			}}},
			want: []string{"backends/echo-abc", "container tcp", "ImagePullBackOff", "toomanyrequests"},
		},
		{
			name: "crashed container",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "frps",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
			}}},
			want: []string{"container frps terminated", "exit 1"},
		},
		{
			name:   "not scheduled",
			status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Message: "0/1 nodes are available"}}},
			want:   []string{"not scheduled", "0/1 nodes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podProblem(pod(tt.status))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("podProblem() = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}
