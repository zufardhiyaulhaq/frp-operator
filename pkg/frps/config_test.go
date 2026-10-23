package frps

import (
	"strings"
	"testing"
)

func TestRender_Minimal(t *testing.T) {
	got, err := Render(Config{Ports: &Ports{Bind: 7000}, Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	want := "log.to = \"console\"\n" +
		"log.level = \"info\"\n" +
		"bindAddr = \"0.0.0.0\"\n" +
		"bindPort = 7000\n" +
		"auth.method = \"token\"\n" +
		"auth.token = \"t\"\n" +
		"allowPorts = [\n" +
		"  { start = 2, end = 1 },\n" +
		"]\n"
	if got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRender_EveryField(t *testing.T) {
	got, err := Render(Config{
		Ports: &Ports{Bind: 7000, KCP: 7000, QUIC: 7001, HTTP: 80, HTTPS: 443, TCPMux: 5002,
			AllowTCP: []int32{8000, 8001, 8002}, AllowUDP: []int32{5353}, WebServer: true},
		Token:         `a"b\c`,
		TLS:           &TLSConfig{Force: true, CertFile: "/etc/frps/tls/tls.crt", KeyFile: "/etc/frps/tls/tls.key", TrustedCAFile: "/etc/frps/tls/ca.crt"},
		SubdomainHost: "example.com",
		WebServer: &WebServerConfig{Port: 7500, User: "admin", Password: "p@ss", Prometheus: true,
			CertFile: "/etc/frps/dashboard-tls/tls.crt", KeyFile: "/etc/frps/dashboard-tls/tls.key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"kcpBindPort = 7000", "quicBindPort = 7001",
		`auth.token = "a\"b\\c"`,
		"transport.tls.force = true",
		`transport.tls.certFile = "/etc/frps/tls/tls.crt"`, `transport.tls.keyFile = "/etc/frps/tls/tls.key"`,
		`transport.tls.trustedCaFile = "/etc/frps/tls/ca.crt"`,
		"vhostHTTPPort = 80", "vhostHTTPSPort = 443", "tcpmuxHTTPConnectPort = 5002", `subDomainHost = "example.com"`,
		"  { start = 8000, end = 8002 },", "  { single = 5353 },",
		`webServer.addr = "0.0.0.0"`, "webServer.port = 7500", `webServer.user = "admin"`, `webServer.password = "p@ss"`,
		`webServer.tls.certFile = "/etc/frps/dashboard-tls/tls.crt"`, `webServer.tls.keyFile = "/etc/frps/dashboard-tls/tls.key"`,
		"enablePrometheus = true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() is missing %q:\n%s", want, got)
		}
	}
}

func TestRender_TLSWithoutCertificate(t *testing.T) {
	got, err := Render(Config{Ports: &Ports{Bind: 7000}, Token: "t", TLS: &TLSConfig{Force: false}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "transport.tls.force = false") || strings.Contains(got, "certFile") || strings.Contains(got, "trustedCaFile") {
		t.Errorf("Render() =\n%s", got)
	}
}

func TestRender_LogLevel(t *testing.T) {
	got, err := Render(Config{Ports: &Ports{Bind: 7000}, Token: "t", LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `log.to = "console"`) || !strings.Contains(got, `log.level = "debug"`) {
		t.Errorf("Render() =\n%s", got)
	}
}
