//go:build e2e

package e2e

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func trustCA(secret string) *frpv1alpha1.ClientSpec_Server_TLS {
	return &frpv1alpha1.ClientSpec_Server_TLS{
		Enable:        true,
		TrustedCAFile: &frpv1alpha1.ConfigMapOrSecretRef{Secret: &frpv1alpha1.Secret{Name: secret, Key: "ca.crt"}},
	}
}

// TestTLSServerVerification: a Client that trusts the right CA connects; one that trusts an
// unrelated CA is rejected, which proves verification is really on.
func TestTLSServerVerification(t *testing.T) {
	c := newCase(t, "tls")
	c.Secret("frps-ca", map[string][]byte{"ca.crt": env.Certs.CA})
	c.Secret("wrong-ca", map[string][]byte{"ca.crt": env.Certs.OtherCA})

	c.Client("trusted", func(s *frpv1alpha1.ClientSpec_Server) { s.TLS = trustCA("frps-ca") })
	proxy := c.Upstream("trusted", "echo", tcpEcho(portTLS))
	c.Client("untrusted", func(s *frpv1alpha1.ClientSpec_Server) { s.TLS = trustCA("wrong-ca") })

	c.waitClientSynced("trusted")
	c.waitProxyRunning("trusted", proxy)
	expectTCPEcho(t, env.FrpsIP, portTLS)
	c.waitFrpcLog("untrusted", "x509")
}

// TestMutualTLS: frps-mtls forces client certificates. A Client with one connects; a Client
// without one is rejected.
func TestMutualTLS(t *testing.T) {
	c := newCase(t, "mtls")
	c.Secret("client-tls", map[string][]byte{"tls.crt": env.Certs.ClientCert, "tls.key": env.Certs.ClientKey, "ca.crt": env.Certs.CA})
	c.Secret("frps-ca", map[string][]byte{"ca.crt": env.Certs.CA})
	ref := func(key string) *frpv1alpha1.SecretRef {
		return &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-tls", Key: key}}
	}

	c.Client("with-cert", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = frpsMTLSHost
		s.TLS = trustCA("client-tls")
		s.TLS.CertFile, s.TLS.KeyFile = ref("tls.crt"), ref("tls.key")
	})
	proxy := c.Upstream("with-cert", "echo", tcpEcho(portMTLS))
	c.Client("without-cert", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = frpsMTLSHost
		s.TLS = trustCA("frps-ca")
	})

	c.waitClientSynced("with-cert")
	c.waitProxyRunning("with-cert", proxy)
	expectTCPEcho(t, env.FrpsMTLSIP, portMTLS)
	// frps rejects the missing client certificate after the TLS 1.3 handshake, so frpc reports
	// a closed session rather than the TLS alert; the login failing is what proves the rejection.
	c.waitFrpcLog("without-cert", "login to the server failed")
}
