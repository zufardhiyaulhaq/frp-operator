//go:build e2e

package e2e

import (
	"context"
	"crypto/x509"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestServerConnectsClient: a Server with a generated token and a dashboard serves a plain
// Client pointed at status.address; a spec change restarts frps and the Client reconnects.
func TestServerConnectsClient(t *testing.T) {
	c := newCase(t, "server")
	c.Server("public", func(s *frpv1alpha1.ServerSpec) {
		s.AllowPorts = &frpv1alpha1.ServerSpec_AllowPorts{TCP: []frpv1alpha1.PortRange{portRange(portServer)}}
		s.Vhost = &frpv1alpha1.ServerSpec_Vhost{HTTPPort: ptr(int32(80))} // below 1024: the non-root sysctl path
		s.Dashboard = &frpv1alpha1.ServerSpec_Dashboard{Enabled: true}
	})
	srv := c.waitServer("public", "Ready", metav1.ConditionTrue, "")
	if want := "public-frps." + c.NS + ".svc"; srv.Status.Address != want || srv.Status.ConnectionSecret != "public-connection" {
		t.Fatalf("status = %+v, want address %s", srv.Status, want)
	}

	// The private side: a plain Client pointed at status.address with the generated token.
	c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = srv.Status.Address
		s.Authentication.Token.Secret = frpv1alpha1.Secret{Name: srv.Status.ConnectionSecret, Key: "token"}
	})
	proxy := c.Upstream("frpc", "echo", tcpEcho(portServer))
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, srv.Status.Address, portServer)

	// frps listens on port 80 as a non-root user: an unknown host gets frps's 404.
	expectHTTPStatus(t, "frps vhost on port 80", fmt.Sprintf("http://%s.cluster.local/", srv.Status.Address), "404")

	// The dashboard answers only with the generated login.
	web := fmt.Sprintf("http://public-frps-web.%s.svc.cluster.local:7500/api/serverinfo", c.NS)
	user, pass := c.secretValue("public-dashboard", "username"), c.secretValue("public-dashboard", "password")
	expectHTTPStatus(t, "dashboard with the login", fmt.Sprintf("-u %s:%s %s", user, pass, web), "200")
	expectHTTPStatus(t, "dashboard without a login", web, "401")

	// A spec change restarts frps; the Client reconnects on its own.
	before := c.serverPod("public").UID
	c.updateServer("public", func(s *frpv1alpha1.ServerSpec) {
		s.AllowPorts.TCP = append(s.AllowPorts.TCP, portRange(portServerAdded))
	})
	eventually(t, 2*time.Minute, "frps pod replaced", func(ctx context.Context) error {
		p := &corev1.Pod{}
		if err := env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: "public-frps"}, p); err != nil {
			return err
		}
		if p.UID == before {
			return fmt.Errorf("still the old pod")
		}
		return nil
	})
	c.waitServer("public", "Ready", metav1.ConditionTrue, "")
	added := c.Upstream("frpc", "echo-added", tcpEcho(portServerAdded))
	c.waitProxyRunning("frpc", added)
	expectTCPEcho(t, srv.Status.Address, portServer)
	expectTCPEcho(t, srv.Status.Address, portServerAdded)
}

// TestServerOwnCertificate: a Server with your certificate publishes its DNS name; a Client that
// dials the Service name (not in the certificate) verifies frps through tls.serverName.
func TestServerOwnCertificate(t *testing.T) {
	c := newCase(t, "server-tls")
	ca, caKey, caPEM, err := newCA("server-tls CA")
	must(t, err)
	crt, key, err := newLeaf(ca, caKey, "frps", []string{"frps.e2e.internal"}, x509.ExtKeyUsageServerAuth)
	must(t, err)
	c.Secret("frps-cert", map[string][]byte{"tls.crt": crt, "tls.key": key, "ca.crt": caPEM})
	c.Server("public", func(s *frpv1alpha1.ServerSpec) {
		s.Bind.TLS = &frpv1alpha1.ServerSpec_Bind_TLS{Certificate: &frpv1alpha1.SecretNameRef{Secret: frpv1alpha1.SecretName{Name: "frps-cert"}}}
		s.Authentication = &frpv1alpha1.ServerSpec_Authentication{
			Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "frps-token", Key: "token"}},
		}
		s.AllowPorts = &frpv1alpha1.ServerSpec_AllowPorts{TCP: []frpv1alpha1.PortRange{portRange(portServerTLS)}}
	})
	srv := c.waitServer("public", "Ready", metav1.ConditionTrue, "")
	if srv.Status.TLS == nil || !slices.Contains(srv.Status.TLS.ServerNames, "frps.e2e.internal") {
		t.Fatalf("status.tls = %+v", srv.Status.TLS)
	}

	c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = srv.Status.Address // not in the certificate: serverName is what makes verification pass
		s.TLS = &frpv1alpha1.ClientSpec_Server_TLS{
			Enable:        true,
			ServerName:    "frps.e2e.internal",
			TrustedCAFile: &frpv1alpha1.ConfigMapOrSecretRef{Secret: &frpv1alpha1.Secret{Name: srv.Status.ConnectionSecret, Key: "ca.crt"}},
		}
	})
	proxy := c.Upstream("frpc", "echo", tcpEcho(portServerTLS))
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, srv.Status.Address, portServerTLS)
}

// TestServerInvalidConfig: a token Secret without the referenced key is an InvalidConfig with a
// Warning event; fixing the Secret brings the Server up.
func TestServerInvalidConfig(t *testing.T) {
	c := newCase(t, "server-invalid")
	c.Secret("broken-token", map[string][]byte{"other": []byte("x")})
	c.Server("public", func(s *frpv1alpha1.ServerSpec) {
		s.Authentication = &frpv1alpha1.ServerSpec_Authentication{
			Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "broken-token", Key: "token"}},
		}
	})
	srv := c.waitServer("public", "ConfigSynced", metav1.ConditionFalse, "InvalidConfig")
	if cond := meta.FindStatusCondition(srv.Status.Conditions, "ConfigSynced"); !strings.Contains(cond.Message, `key "token" not found`) {
		t.Errorf("ConfigSynced message = %q", cond.Message)
	}
	c.waitObjectEvent("public", "InvalidConfig")

	secret := &corev1.Secret{}
	must(t, env.Kube.Get(context.Background(), client.ObjectKey{Namespace: c.NS, Name: "broken-token"}, secret))
	secret.Data["token"] = []byte(frpsToken)
	must(t, env.Kube.Update(context.Background(), secret))
	c.waitServer("public", "Ready", metav1.ConditionTrue, "")
}
