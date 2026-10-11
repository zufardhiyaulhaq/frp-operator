//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// onlyEgressViaProxy lets pods in the case namespace reach DNS, the egress proxy and the
// backends — and nothing else, so frps is only reachable through the proxy.
func (c *Case) onlyEgressViaProxy() {
	c.t.Helper()
	udp, tcp, dns := corev1.ProtocolUDP, corev1.ProtocolTCP, intstr.FromInt32(53)
	to := func(namespace string) []networkingv1.NetworkPolicyPeer {
		return []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
		}}}
	}
	must(c.t, env.Kube.Create(context.Background(), &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "only-via-egress-proxy", Namespace: c.NS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: to("kube-system"), Ports: []networkingv1.NetworkPolicyPort{{Protocol: &udp, Port: &dns}, {Protocol: &tcp, Port: &dns}}},
				{To: to(egressNamespace)},
				{To: to(backendsNamespace)},
			},
		},
	}))
}

// probeEgressBlocked proves the policy is enforced from inside the case namespace: the egress
// proxy is reachable and frps is not.
func (c *Case) probeEgressBlocked() {
	c.t.Helper()
	ctx := context.Background()
	must(c.t, env.Kube.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: c.NS},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "probe", Image: "nicolaka/netshoot:v0.14", Command: []string{"sleep", "infinity"},
		}}},
	}))
	eventually(c.t, 2*time.Minute, "probe pod ready", func(ctx context.Context) error {
		p := &corev1.Pod{}
		if err := env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: "probe"}, p); err != nil {
			return err
		}
		if !podReady(p) {
			return fmt.Errorf("%s", podProblem(p))
		}
		return nil
	})
	eventually(c.t, 30*time.Second, "egress proxy reachable from the case namespace", func(ctx context.Context) error {
		_, err := env.exec(ctx, c.NS, "probe", "probe", "nc", "-z", "-w", "3", egressProxyHost, "3128")
		return err
	})
	eventually(c.t, 30*time.Second, "frps blocked from the case namespace", func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			if _, err := env.exec(ctx, c.NS, "probe", "probe", "nc", "-z", "-w", "3", frpsMainHost, "7000"); err == nil {
				return fmt.Errorf("frps is reachable directly: the NetworkPolicy is not enforced")
			}
		}
		return nil
	})
}

// TestEgressProxy runs frpc behind an http and a socks5 egress proxy with Secret-sourced
// credentials. frps is unreachable directly, so working traffic proves the proxy carried it.
func TestEgressProxy(t *testing.T) {
	t.Parallel() // without it the parent blocks every other top-level test until its subtests finish
	cases := []struct {
		name       string
		port       int
		remotePort int
		logType    string // 3proxy's "proxy.type" for this listener
	}{
		{name: "http", port: 3128, remotePort: portProxyHTTP, logType: "PROXY"},
		{name: "socks5", port: 1080, remotePort: portProxySOCKS, logType: "SOCK5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCase(t, "proxy-"+tc.name)
			c.onlyEgressViaProxy()
			c.probeEgressBlocked()
			c.Secret("egress-proxy", map[string][]byte{"username": []byte(proxyUser), "password": []byte(proxyPassword)})
			c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
				s.Transport = &frpv1alpha1.ClientSpec_Server_Transport{
					ProxyURL: fmt.Sprintf("%s://%s:%d", tc.name, egressProxyHost, tc.port),
					ProxyCredentials: &frpv1alpha1.ClientSpec_Server_Transport_ProxyCredentials{
						Username: frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "egress-proxy", Key: "username"}},
						Password: frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "egress-proxy", Key: "password"}},
					},
				}
			})
			proxy := c.Upstream("frpc", "echo", tcpEcho(tc.remotePort))

			c.waitClientSynced("frpc")
			c.waitProxyRunning("frpc", proxy)
			expectTCPEcho(t, env.FrpsIP, tc.remotePort)

			// 3proxy logs a connection only when it closes, and frpc's tunnel to frps is long-lived:
			// delete the frpc pod (the operator recreates it) to close the tunnel, then expect an
			// authenticated connection to frps's bind port in the proxy log.
			must(t, env.Kube.Delete(context.Background(), c.pod("frpc")))
			eventually(t, 60*time.Second, "egress proxy logged the frps connection", func(ctx context.Context) error {
				pods, err := env.Clientset.CoreV1().Pods(egressNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=egress-proxy"})
				if err != nil || len(pods.Items) == 0 {
					return fmt.Errorf("egress proxy pod: %v", err)
				}
				out, err := env.logs(ctx, egressNamespace, pods.Items[0].Name, "proxy")
				if err != nil {
					return err
				}
				for _, line := range strings.Split(out, "\n") {
					if strings.Contains(line, `"type":"`+tc.logType+`"`) && strings.Contains(line, `"user":"`+proxyUser+`"`) && strings.Contains(line, `"port":7000`) {
						return nil
					}
				}
				return fmt.Errorf("no %s log line for user %q to port 7000", tc.logType, proxyUser)
			})
		})
	}
}
