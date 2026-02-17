package v1alpha1

type Secret struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type SecretRef struct {
	Secret Secret `json:"secret"`
}

type ConfigMapRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ConfigMapOrSecretRef struct {
	// +optional
	Secret *Secret `json:"secret,omitempty"`
	// +optional
	ConfigMap *ConfigMapRef `json:"configMap,omitempty"`
}
