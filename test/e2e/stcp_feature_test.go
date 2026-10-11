//go:build e2e

package e2e

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestSTCPVisitor publishes the TCP echo backend as STCP on one Client and reaches it through a
// visitor on a second Client, via the visitor port on that Client's Service.
func TestSTCPVisitor(t *testing.T) {
	c := newCase(t, "stcp")
	c.Secret("stcp-key", map[string][]byte{"key": []byte("e2e-stcp-secret")})
	key := frpv1alpha1.Secret{Name: "stcp-key", Key: "key"}

	c.Client("server", nil)
	proxy := c.Upstream("server", "private", frpv1alpha1.UpstreamSpec{STCP: &frpv1alpha1.UpstreamSpec_STCP{
		Host: tcpEchoHost, Port: 9000, SecretKey: frpv1alpha1.UpstreamSpec_STCP_SecretKey{Secret: key},
	}})
	c.Client("visitor", nil)
	c.Visitor("visitor", "private-visitor", frpv1alpha1.VisitorSpec{STCP: &frpv1alpha1.VisitorSpec_STCP{
		Host: "0.0.0.0", Port: 9100, ServerName: proxy,
		ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{Secret: key},
	}})

	c.waitClientSynced("server")
	c.waitProxyRunning("server", proxy)
	c.waitClientSynced("visitor")
	expectTCPEcho(t, c.clientService("visitor"), 9100)
}
