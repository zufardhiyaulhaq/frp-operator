//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func restarts(p *corev1.Pod) int32 {
	var n int32
	for _, cs := range p.Status.ContainerStatuses {
		n += cs.RestartCount
	}
	return n
}

// TestReloadWithoutRestart adds an Upstream to a running Client: the new proxy must come up
// through a config reload, with the same frpc pod and no container restart.
func TestReloadWithoutRestart(t *testing.T) {
	c := newCase(t, "reload")
	c.Client("frpc", nil)
	first := c.Upstream("frpc", "first", tcpEcho(portReloadFirst))
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", first)
	before := c.pod("frpc")

	second := c.Upstream("frpc", "second", tcpEcho(portReloadSecond))
	c.waitProxyRunning("frpc", second)
	expectTCPEcho(t, env.FrpsIP, portReloadSecond)
	expectTCPEcho(t, env.FrpsIP, portReloadFirst)

	after := c.pod("frpc")
	if after.UID != before.UID {
		t.Fatalf("frpc pod was replaced (%s -> %s), want an in-place reload", before.UID, after.UID)
	}
	if n := restarts(after); n != 0 {
		t.Fatalf("frpc container restarted %d times, want 0", n)
	}
}

// TestInvalidConfigThenRecovery: a token Secret without the referenced key must surface as
// InvalidConfig in status, events and metrics; fixing the Secret must bring the Client back.
func TestInvalidConfigThenRecovery(t *testing.T) {
	c := newCase(t, "invalid")
	ctx := context.Background()
	c.Secret("broken-token", map[string][]byte{"other": []byte("x")})
	c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Authentication.Token.Secret = frpv1alpha1.Secret{Name: "broken-token", Key: "token"}
	})
	proxy := c.Upstream("frpc", "echo", tcpEcho(portInvalid))

	cond := c.waitCondition("frpc", "ConfigSynced", metav1.ConditionFalse, "InvalidConfig")
	if !strings.Contains(cond.Message, `key "token" not found`) {
		t.Errorf("ConfigSynced message = %q, want it to name the missing key", cond.Message)
	}
	c.waitEvent("frpc", "InvalidConfig")
	c.waitMetric("frp_client_config_synced", map[string]string{"namespace": c.NS, "client": c.clientName("frpc")}, 0)

	secret := &corev1.Secret{}
	must(t, env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: "broken-token"}, secret))
	secret.Data["token"] = []byte(frpsToken)
	must(t, env.Kube.Update(ctx, secret))

	c.waitClientSynced("frpc")
	c.waitMetric("frp_client_config_synced", map[string]string{"namespace": c.NS, "client": c.clientName("frpc")}, 1)
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, env.FrpsIP, portInvalid)
}
