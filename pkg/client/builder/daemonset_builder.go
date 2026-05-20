package builder

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// DaemonSetBuilder emits an appsv1.DaemonSet whose pod template mirrors the
// bare-Pod path. Used when Client.spec.workload.kind == DaemonSet to run one
// frpc per matching node — typically paired with Upstream tcp.loadBalancer.group
// so frps round-robins traffic across the registered DaemonSet pods.
type DaemonSetBuilder struct {
	PodBuilder
}

func NewDaemonSetBuilder() *DaemonSetBuilder {
	return &DaemonSetBuilder{}
}

func (n *DaemonSetBuilder) SetName(name string) *DaemonSetBuilder {
	n.Name = name
	return n
}

func (n *DaemonSetBuilder) SetNamespace(namespace string) *DaemonSetBuilder {
	n.Namespace = namespace
	return n
}

func (n *DaemonSetBuilder) SetImage(image string) *DaemonSetBuilder {
	n.Image = image
	return n
}

func (n *DaemonSetBuilder) SetPodTemplate(podTemplate *frpv1alpha1.ClientSpec_PodTemplate) *DaemonSetBuilder {
	n.PodTemplate = podTemplate
	return n
}

func (n *DaemonSetBuilder) SetTLSSecret(tlsSecret string) *DaemonSetBuilder {
	n.TLSSecret = tlsSecret
	return n
}

func (n *DaemonSetBuilder) SetTLSCAConfigMap(tlsCAConfigMap string) *DaemonSetBuilder {
	n.TLSCAConfigMap = tlsCAConfigMap
	return n
}

func (n *DaemonSetBuilder) Build() (*appsv1.DaemonSet, error) {
	template := n.BuildPodTemplateSpec()

	// Selector must be a stable subset of the pod template labels — pod-template
	// labels injected via Client.spec.podTemplate can change between releases
	// and would break the immutable DaemonSet selector invariant. BuildLabels()
	// returns only the operator-managed identity labels.
	selector := n.BuildLabels()

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name + "-frpc",
			Namespace: n.Namespace,
			Labels:    template.ObjectMeta.Labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: template,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}, nil
}
