//go:build e2e

package e2e

import "testing"

// TestUDP sends a datagram through frps-main to the UDP echo backend.
func TestUDP(t *testing.T) {
	c := newCase(t, "udp")
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "echo", udpEcho(portUDP))

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectUDPEcho(t, env.FrpsIP, portUDP)
}

// TestDisabledUpstream checks that enabled:false leaves the proxy off while its sibling works.
func TestDisabledUpstream(t *testing.T) {
	c := newCase(t, "disabled")
	c.Client("frpc", nil)
	on := c.Upstream("frpc", "on", tcpEcho(portEnabled))
	off := tcpEcho(portDisabled)
	off.Enabled = ptr(false)
	c.Upstream("frpc", "off", off)

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", on)
	expectTCPEcho(t, env.FrpsIP, portEnabled)
	expectNoTCP(t, env.FrpsIP, portDisabled)
}
