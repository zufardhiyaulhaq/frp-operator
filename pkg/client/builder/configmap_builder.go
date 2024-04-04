package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigMapBuilder struct {
	Name      string
	Namespace string
	Config    string
}

func NewConfigMapBuilder() *ConfigMapBuilder {
	return &ConfigMapBuilder{}
}

func (n *ConfigMapBuilder) SetConfig(config string) *ConfigMapBuilder {
	n.Config = config
	return n
}

func (n *ConfigMapBuilder) SetName(name string) *ConfigMapBuilder {
	n.Name = name
	return n
}

func (n *ConfigMapBuilder) SetNamespace(namespace string) *ConfigMapBuilder {
	n.Namespace = namespace
	return n
}

func (n *ConfigMapBuilder) Build() (*corev1.ConfigMap, error) {
	data := make(map[string]string)
	data["config.toml"] = n.Config

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name + "-frpc-config",
			Namespace: n.Namespace,
			Labels: map[string]string{
				"app":       n.Name,
				"generated": "frp-operator",
			},
		},
		Data: data,
	}

	return configMap, nil
}

func (n *ConfigMapBuilder) BuildLabels() map[string]string {
	var labels = map[string]string{
		"app.kubernetes.io/name":       n.Name + "-frpc-config",
		"app.kubernetes.io/managed-by": "frp-operator",
		"app.kubernetes.io/created-by": n.Name,
	}

	return labels
}
