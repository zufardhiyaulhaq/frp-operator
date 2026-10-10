package e2e

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// podProblem says why a pod is not ready, so a setup timeout names the cause (for example
// ImagePullBackOff from a Docker Hub rate limit) instead of only "timed out".
func podProblem(p *corev1.Pod) string {
	where := p.Namespace + "/" + p.Name
	statuses := append(append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...), p.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if w := cs.State.Waiting; w != nil {
			return fmt.Sprintf("%s: container %s waiting: %s %s", where, cs.Name, w.Reason, w.Message)
		}
		if term := cs.State.Terminated; term != nil {
			return fmt.Sprintf("%s: container %s terminated: %s (exit %d)", where, cs.Name, term.Reason, term.ExitCode)
		}
		if !cs.Ready {
			return fmt.Sprintf("%s: container %s not ready", where, cs.Name)
		}
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status != corev1.ConditionTrue {
			return fmt.Sprintf("%s: not scheduled: %s", where, c.Message)
		}
	}
	return fmt.Sprintf("%s: phase %s", where, p.Status.Phase)
}
