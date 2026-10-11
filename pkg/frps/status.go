package frps

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// Condition reasons (condition types are status.ConditionTypeReady / ConditionTypeConfigSync).
const (
	ReasonInvalidConfig      = "InvalidConfig"
	ReasonConfigApplied      = "ConfigApplied"
	ReasonRestarting         = "Restarting"
	ReasonRestartPending     = "RestartPending"
	ReasonPodStarting        = "PodStarting"
	ReasonPodFailed          = "PodFailed"
	ReasonAddressPending     = "AddressPending"
	ReasonLoadBalancerFailed = "LoadBalancerFailed"
	ReasonReady              = "Ready"
)

// Event reasons.
const (
	ReasonPodCreated                = "PodCreated"
	ReasonTunnelTLSUnverified       = "TunnelTLSUnverified"
	ReasonDashboardInsecure         = "DashboardInsecure"
	ReasonTokenRotated              = "TokenRotated"
	ReasonServiceMonitorUnavailable = "ServiceMonitorUnavailable"
)

// PodState reports whether the frps pod is ready, or what is wrong with it.
func PodState(p *corev1.Pod) (ready bool, problem string) {
	if p.Status.Phase == corev1.PodFailed {
		return false, fmt.Sprintf("frps pod failed: %s %s", p.Status.Reason, p.Status.Message)
	}
	for _, cs := range p.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" && w.Reason != "ContainerCreating" && w.Reason != "PodInitializing" {
			return false, fmt.Sprintf("container %s: %s: %s", cs.Name, w.Reason, w.Message)
		}
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true, ""
		}
	}
	return false, ""
}
