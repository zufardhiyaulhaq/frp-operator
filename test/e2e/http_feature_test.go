//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func expectHTTPStatus(t *testing.T, what, args, want string) {
	t.Helper()
	eventually(t, 30*time.Second, what, func(ctx context.Context) error {
		code, err := curl(ctx, "-o /dev/null -w '%{http_code}' "+args)
		if err != nil {
			return err
		}
		if code != want {
			return fmt.Errorf("status %s, want %s", code, want)
		}
		return nil
	})
}

func expectBody(t *testing.T, what, args string, wants ...string) {
	t.Helper()
	eventually(t, 30*time.Second, what, func(ctx context.Context) error {
		out, err := curl(ctx, args)
		if err != nil {
			return err
		}
		for _, w := range wants {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(w)) {
				return fmt.Errorf("response lacks %q:\n%s", w, out)
			}
		}
		return nil
	})
}

// TestHTTPVhost covers customDomains, locations, request/response headers, hostHeaderRewrite
// and basic auth on frps's vhost HTTP port.
func TestHTTPVhost(t *testing.T) {
	c := newCase(t, "http")
	c.Secret("basic-auth", map[string][]byte{"user": []byte("alice"), "password": []byte("s3cret")})
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "web", frpv1alpha1.UpstreamSpec{HTTP: &frpv1alpha1.UpstreamSpec_HTTP{
		Host:              httpEchoHost,
		Port:              80,
		CustomDomains:     []string{domain},
		Locations:         []string{"/api"},
		HostHeaderRewrite: "internal.e2e",
		RequestHeaders:    &frpv1alpha1.HTTPHeaders{Set: map[string]string{"x-e2e": "from-frp"}},
		ResponseHeaders:   &frpv1alpha1.HTTPHeaders{Set: map[string]string{"x-e2e-response": "yes"}},
		HTTPUser:          &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "basic-auth", Key: "user"}},
		HTTPPassword:      &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "basic-auth", Key: "password"}},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	base := fmt.Sprintf("-H 'Host: %s' http://%s:8080", domain, env.FrpsIP)
	expectHTTPStatus(t, "basic auth is enforced", base+"/api/ping", "401")
	expectBody(t, "request reaches the backend rewritten", "-i -u alice:s3cret "+base+"/api/ping",
		"HTTP/1.1 200", `"path": "/api/ping"`, `"host": "internal.e2e"`, `"x-e2e": "from-frp"`, "x-e2e-response: yes")
	expectHTTPStatus(t, "paths outside locations are not routed", "-u alice:s3cret "+base+"/other", "404")
}

// TestHTTPSVhost routes TLS by SNI on frps's vhost HTTPS port to the backend's own TLS.
func TestHTTPSVhost(t *testing.T) {
	c := newCase(t, "https")
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "secure", frpv1alpha1.UpstreamSpec{HTTPS: &frpv1alpha1.UpstreamSpec_HTTPS{
		Host: httpEchoHost, Port: 443, CustomDomains: []string{domain},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	expectBody(t, "TLS passes through to the backend",
		fmt.Sprintf("-k --resolve %s:8443:%s https://%s:8443/hello", domain, env.FrpsIP, domain), `"path": "/hello"`)
}

// TestTCPMUX tunnels through frps's HTTP CONNECT multiplexer, routed by the CONNECT host.
func TestTCPMUX(t *testing.T) {
	c := newCase(t, "tcpmux")
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "mux", frpv1alpha1.UpstreamSpec{TCPMUX: &frpv1alpha1.UpstreamSpec_TCPMUX{
		Host: httpEchoHost, Port: 80, Multiplexer: "httpconnect", CustomDomains: []string{domain},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	expectBody(t, "CONNECT tunnel reaches the backend",
		fmt.Sprintf("--proxytunnel -x http://%s:5002 http://%s/hello", env.FrpsIP, domain), `"path": "/hello"`)
}
