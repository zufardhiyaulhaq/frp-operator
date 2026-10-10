// Package e2e is the end-to-end suite for frp-operator. The suite runs behind the `e2e` build
// tag (`make test-e2e`); the untagged files hold pure helpers that `make test` unit-tests.
package e2e

import (
	"fmt"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// ContextName is the only kubeconfig context the suite talks to: the k3d cluster `make e2e-up`
// creates.
const ContextName = "k3d-frp-operator-e2e"

// requireE2EContext refuses any kubeconfig except the one `make e2e-up` writes: exactly one
// context, named ContextName, and current. The suite creates namespaces and NetworkPolicies, so
// it must never reach another cluster by accident.
func requireE2EContext(cfg *clientcmdapi.Config) error {
	if cfg.CurrentContext != ContextName {
		return fmt.Errorf("refusing to run: current kubeconfig context is %q, want %q", cfg.CurrentContext, ContextName)
	}
	if _, ok := cfg.Contexts[ContextName]; !ok {
		return fmt.Errorf("refusing to run: context %q is not defined in the kubeconfig", ContextName)
	}
	if len(cfg.Contexts) != 1 {
		return fmt.Errorf("refusing to run: kubeconfig has %d contexts, want only %q (use the file written by make e2e-up, not ~/.kube/config)", len(cfg.Contexts), ContextName)
	}
	return nil
}
