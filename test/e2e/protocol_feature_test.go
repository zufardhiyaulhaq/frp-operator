//go:build e2e

package e2e

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestServerProtocols connects frpc to frps over each non-default transport and sends traffic.
func TestServerProtocols(t *testing.T) {
	t.Parallel() // without it the parent blocks every other top-level test until its subtests finish
	cases := []struct {
		name, host string
		port       int
		remotePort int
	}{
		{name: "kcp", host: frpsMainHost, port: 7000, remotePort: portKCP},
		{name: "quic", host: frpsMainHost, port: 7001, remotePort: portQUIC},
		{name: "websocket", host: frpsMainHost, port: 7000, remotePort: portWebsocket},
		{name: "wss", host: wssHost, port: 7443, remotePort: portWSS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCase(t, "proto-"+tc.name)
			c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
				s.Host, s.Port, s.Protocol = tc.host, tc.port, ptr(tc.name)
			})
			proxy := c.Upstream("frpc", "echo", tcpEcho(tc.remotePort))

			c.waitClientSynced("frpc")
			c.waitProxyRunning("frpc", proxy)
			expectTCPEcho(t, env.FrpsIP, tc.remotePort)
		})
	}
}
