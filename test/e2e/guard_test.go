package e2e

import (
	"strings"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func kubeconfig(current string, contexts ...string) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	for _, name := range contexts {
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: name, AuthInfo: name}
	}
	cfg.CurrentContext = current
	return cfg
}

func TestRequireE2EContext(t *testing.T) {
	prod := "arn:aws:eks:us-east-2:111111111111:cluster/prod"
	tests := []struct {
		name    string
		cfg     *clientcmdapi.Config
		wantErr string
	}{
		{name: "the e2e cluster only", cfg: kubeconfig(ContextName, ContextName)},
		{name: "a production context is current", cfg: kubeconfig(prod, ContextName, prod), wantErr: "current kubeconfig context"},
		{name: "e2e is current but other contexts exist", cfg: kubeconfig(ContextName, ContextName, "orbstack"), wantErr: "2 contexts"},
		{name: "current context is not defined", cfg: kubeconfig(ContextName), wantErr: "not defined"},
		{name: "empty kubeconfig", cfg: kubeconfig(""), wantErr: "current kubeconfig context"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireE2EContext(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("requireE2EContext() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("requireE2EContext() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
