//go:build e2e

package e2e

import "testing"

// TestTCP exposes the TCP echo backend through frps-main and sends a line through it.
func TestTCP(t *testing.T) {
	c := newCase(t, "tcp")
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "echo", tcpEcho(portTCP))

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, env.FrpsIP, portTCP)
}
