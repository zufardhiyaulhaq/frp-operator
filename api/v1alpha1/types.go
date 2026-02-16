package v1alpha1

type Secret struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type SecretRef struct {
	Secret Secret `json:"secret"`
}
