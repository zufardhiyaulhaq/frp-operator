//go:build frpcverify

// Package frpcconfig renders frpc.toml for every feature, every example and the ServerPool
// path through the real CR → model → template pipeline, then validates each file with the
// real frpc binary (`frpc verify`, strict mode) from the image the operator deploys.
//
// Run with: make test-frpc-config (requires Docker).
package frpcconfig

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/builder"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/loadbalancer"
)

const ns = "default"

// rendered is one generated frpc.toml plus the substrings it must contain.
type rendered struct {
	name   string
	config string
	expect []string
}

func TestGeneratedConfigsPassFrpcVerify(t *testing.T) {
	image := os.Getenv("FRPC_IMAGE")
	if image == "" {
		t.Fatal("FRPC_IMAGE is not set; run via `make test-frpc-config`")
	}

	var all []rendered
	all = append(all, renderFeatureCases(t)...)
	examples := loadExamples(t)
	all = append(all, renderExamples(t, examples)...)
	all = append(all, renderServerPool(t, examples["loadbalancer"])...)

	dir := t.TempDir()
	for _, r := range all {
		for _, want := range r.expect {
			if !strings.Contains(r.config, want) {
				t.Errorf("%s: rendered config is missing %q\n%s", r.name, want, r.config)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, r.name+".toml"), []byte(r.config+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	results := frpcVerify(t, image, dir)
	for _, r := range all {
		out, ok := results[r.name+".toml"]
		if !ok {
			t.Errorf("%s: frpc verify produced no result", r.name)
			continue
		}
		if !strings.Contains(out, "syntax is ok") {
			t.Errorf("%s: frpc verify failed: %s\n%s", r.name, strings.TrimSpace(out), r.config)
		}
	}
	t.Logf("verified %d generated configs with %s", len(all), image)
}

var verifyMarker = regexp.MustCompile(`(?m)^### (\S+)$`)

// frpcVerify runs `frpc verify` on every *.toml in dir inside a single container and returns
// the combined output per file name.
func frpcVerify(t *testing.T, image, dir string) map[string]string {
	t.Helper()
	script := `for f in /c/*.toml; do echo "### $(basename "$f")"; /usr/bin/frpc verify -c "$f" 2>&1; done`
	cmd := exec.Command("docker", "run", "--rm", "-v", dir+":/c:ro", "--entrypoint", "sh", image, "-c", script)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("docker run %s: %v\n%s", image, err, out.String())
	}

	results := map[string]string{}
	text := out.String()
	locs := verifyMarker.FindAllStringSubmatchIndex(text, -1)
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		results[text[loc[2]:loc[3]]] = text[loc[1]:end]
	}
	return results
}

func scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1.AddToScheme(s)
	return s
}

func render(t *testing.T, objs []client.Object, c *v1.Client, ups []v1.Upstream, vis []v1.Visitor) (string, error) {
	t.Helper()
	k8s := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(objs...).Build()
	cfg, err := models.NewConfig(k8s, c, ups, vis)
	if err != nil {
		return "", fmt.Errorf("NewConfig: %w", err)
	}
	return builder.NewConfigurationBuilder().SetConfig(cfg).Build()
}

// ---------------------------------------------------------------------------------------
// Feature cases: one CR set per feature, with every optional field filled in.
// ---------------------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

var creds = map[string]string{
	"token":       "TOKEN-VAL",
	"oidc-id":     "OIDC-ID-VAL",
	"oidc-secret": "OIDC-SECRET-VAL",
	"admin-user":  "ADMIN-USER-VAL",
	"admin-pass":  "ADMIN-PASS-VAL",
	"tls-crt":     "x",
	"tls-key":     "x",
	"ca":          "x",
	"groupkey":    "GROUPKEY-VAL",
	"p-user":      "PLUGIN-USER-VAL",
	"p-pass":      "PLUGIN-PASS-VAL",
	"h-user":      "HTTP-USER-VAL",
	"h-pass":      "HTTP-PASS-VAL",
	"stcp-key":    "STCP-KEY-VAL",
	"xtcp-key":    "XTCP-KEY-VAL",
	"special":     `pa"ss\w0rd`,
}

func credsSecret() *corev1.Secret {
	data := map[string][]byte{}
	for k, v := range creds {
		data[k] = []byte(v)
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: ns}, Data: data}
}

func sec(key string) v1.Secret      { return v1.Secret{Name: "creds", Key: key} }
func sref(key string) *v1.SecretRef { return &v1.SecretRef{Secret: sec(key)} }

func baseClient() *v1.Client {
	return &v1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: ns},
		Spec: v1.ClientSpec{Server: v1.ClientSpec_Server{
			Host: "frps.example.com", Port: 7000,
			Authentication: v1.ClientSpec_Server_Authentication{
				Token: &v1.ClientSpec_Server_Authentication_Token{Secret: sec("token")},
			},
		}},
	}
}

func upstream(name string, spec v1.UpstreamSpec) v1.Upstream {
	spec.Client = "c"
	return v1.Upstream{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: spec}
}

func visitor(name string, spec v1.VisitorSpec) v1.Visitor {
	spec.Client = "c"
	return v1.Visitor{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: spec}
}

func tcpBase() *v1.UpstreamSpec_TCP {
	return &v1.UpstreamSpec_TCP{Host: "svc.ns.svc", Port: 8080, Server: v1.UpstreamSpec_TCP_Server{Port: 18080}}
}

func healthCheck() *v1.UpstreamSpec_TCP_HealthCheck {
	return &v1.UpstreamSpec_TCP_HealthCheck{TimeoutSeconds: 3, MaxFailed: 4, IntervalSeconds: 11}
}

var healthCheckExpect = []string{`healthCheck.type = "tcp"`, "healthCheck.timeoutSeconds = 3", "healthCheck.maxFailed = 4", "healthCheck.intervalSeconds = 11"}

func proxyTransport() *v1.UpstreamSpec_TCP_Transport {
	return &v1.UpstreamSpec_TCP_Transport{
		UseEncryption: true, UseCompression: true,
		BandwdithLimit: &v1.UpstreamSpec_TCP_Transport_BandwdithLimit{Enabled: true, Limit: 10, Type: "MB"},
	}
}

var proxyTransportExpect = []string{"transport.useEncryption = true", "transport.useCompression = true", `transport.bandwidthLimit = "10MB"`, `transport.bandwidthLimitMode = "client"`}

type featureCase struct {
	name     string
	client   *v1.Client
	upstream []v1.Upstream
	visitor  []v1.Visitor
	expect   []string
}

func featureCases() []featureCase {
	var cs []featureCase
	add := func(c featureCase) {
		if c.client == nil {
			c.client = baseClient()
		}
		cs = append(cs, c)
	}

	// --- Client ---
	add(featureCase{name: "client_token", expect: []string{`serverAddr = "frps.example.com"`, "serverPort = 7000", `auth.method = "token"`, `auth.token = "TOKEN-VAL"`, `clientID = "default/c"`}})

	c := baseClient()
	c.Spec.Server.Authentication = v1.ClientSpec_Server_Authentication{OIDC: &v1.ClientSpec_Server_Authentication_OIDC{
		ClientID: *sref("oidc-id"), ClientSecret: *sref("oidc-secret"), TokenEndpointURL: "https://idp.example.com/token", Audience: "frps-aud", Scope: "openid profile",
	}}
	add(featureCase{name: "client_oidc", client: c, expect: []string{`auth.method = "oidc"`, `auth.oidc.clientID = "OIDC-ID-VAL"`, `auth.oidc.clientSecret = "OIDC-SECRET-VAL"`, `auth.oidc.tokenEndpointURL = "https://idp.example.com/token"`, `auth.oidc.audience = "frps-aud"`, `auth.oidc.scope = "openid profile"`}})

	c = baseClient()
	c.Spec.Server.Authentication.Token.Secret = sec("special")
	add(featureCase{name: "client_token_special_chars", client: c, expect: []string{`auth.token = "pa\"ss\\w0rd"`}})

	c = baseClient()
	c.Spec.Server.AdminServer = &v1.ClientSpec_Server_AdminServer{Port: 7500, PprofEnable: true,
		Username: &v1.ClientSpec_Server_AdminServer_Username{Secret: sec("admin-user")},
		Password: &v1.ClientSpec_Server_AdminServer_Password{Secret: sec("admin-pass")}}
	add(featureCase{name: "client_admin", client: c, expect: []string{"webServer.port = 7500", `webServer.user = "ADMIN-USER-VAL"`, `webServer.password = "ADMIN-PASS-VAL"`, "webServer.pprofEnable = true"}})

	c = baseClient()
	c.Spec.Server.STUNServer = ptr("stun.example.com:3478")
	add(featureCase{name: "client_stun", client: c, expect: []string{`natHoleStunServer = "stun.example.com:3478"`}})

	c = baseClient()
	c.Spec.Server.TLS = &v1.ClientSpec_Server_TLS{Enable: true, CertFile: sref("tls-crt"), KeyFile: sref("tls-key"), TrustedCAFile: &v1.ConfigMapOrSecretRef{Secret: ptr(sec("ca"))}}
	add(featureCase{name: "client_tls", client: c, expect: []string{"transport.tls.enable = true", `transport.tls.certFile = "/etc/frp/tls/tls.crt"`, `transport.tls.keyFile = "/etc/frp/tls/tls.key"`, `transport.tls.trustedCaFile = "/etc/frp/tls/ca.crt"`}})

	for _, tc := range []struct {
		name string
		mux  bool
		wire string
	}{{"client_transport_tcpmux_false_v1", false, "v1"}, {"client_transport_tcpmux_true_v2", true, "v2"}} {
		c = baseClient()
		c.Spec.Server.Transport = &v1.ClientSpec_Server_Transport{PoolCount: 5, TCPMux: ptr(tc.mux), DialServerTimeout: "15s", DialServerKeepalive: "1m", ConnectServerLocalIP: "10.0.0.5", WireProtocol: tc.wire}
		add(featureCase{name: tc.name, client: c, expect: []string{"transport.poolCount = 5", fmt.Sprintf("transport.tcpMux = %v", tc.mux), "transport.dialServerTimeout = 15", "transport.dialServerKeepalive = 60", `transport.connectServerLocalIP = "10.0.0.5"`, fmt.Sprintf(`transport.wireProtocol = "%s"`, tc.wire)}})
	}
	c = baseClient()
	c.Spec.Server.Transport = &v1.ClientSpec_Server_Transport{DialServerKeepalive: "-1s"}
	add(featureCase{name: "client_transport_keepalive_disabled", client: c, expect: []string{"transport.dialServerKeepalive = -1"}})
	c = baseClient()
	c.Spec.Server.Transport = &v1.ClientSpec_Server_Transport{ProxyURL: "socks5://user:pass@proxy.local:1080"}
	add(featureCase{name: "client_transport_proxy_url", client: c, expect: []string{`transport.proxyURL = "socks5://user:pass@proxy.local:1080"`}})

	for _, proto := range []string{"tcp", "kcp", "quic", "websocket", "wss"} {
		c = baseClient()
		c.Spec.Server.Protocol = ptr(proto)
		var expect []string
		if proto != "tcp" {
			expect = []string{fmt.Sprintf(`transport.protocol = "%s"`, proto)}
		}
		add(featureCase{name: "client_protocol_" + proto, client: c, expect: expect})
	}

	c = baseClient()
	c.Spec.ClientID = ptr("my-custom-id")
	add(featureCase{name: "client_clientid", client: c, expect: []string{`clientID = "my-custom-id"`}})

	// --- TCP ---
	add(featureCase{name: "tcp_basic", upstream: []v1.Upstream{upstream("tcp-basic", v1.UpstreamSpec{TCP: tcpBase()})},
		expect: []string{`name = "tcp-basic"`, `type = "tcp"`, `localIP = "svc.ns.svc"`, "localPort = 8080", "remotePort = 18080"}})
	t := tcpBase()
	t.ProxyProtocol = ptr("v2")
	t.HealthCheck = healthCheck()
	t.Transport = proxyTransport()
	t.LoadBalancer = &v1.LoadBalancer{Group: "g", GroupKey: sref("groupkey")}
	t2 := tcpBase()
	t2.Port = 8081
	t2.LoadBalancer = &v1.LoadBalancer{Group: "g", GroupKey: sref("groupkey")}
	add(featureCase{name: "tcp_maximal", upstream: []v1.Upstream{upstream("tcp-max", v1.UpstreamSpec{TCP: t}), upstream("tcp-max-2", v1.UpstreamSpec{TCP: t2})},
		expect: append(append([]string{`transport.proxyProtocolVersion = "v2"`, `loadBalancer.group = "g"`, `loadBalancer.groupKey = "GROUPKEY-VAL"`}, healthCheckExpect...), proxyTransportExpect...)})
	add(featureCase{name: "tcp_disabled", upstream: []v1.Upstream{upstream("tcp-off", v1.UpstreamSpec{Enabled: ptr(false), TCP: tcpBase()})}, expect: []string{"enabled = false"}})

	plugin := func(name string, p *v1.UpstreamPlugin, expect ...string) {
		add(featureCase{name: "tcp_plugin_" + name,
			upstream: []v1.Upstream{upstream("plugin-"+name, v1.UpstreamSpec{TCP: &v1.UpstreamSpec_TCP{Server: v1.UpstreamSpec_TCP_Server{Port: 19000}, Plugin: p}})},
			expect:   append([]string{fmt.Sprintf(`plugin.type = "%s"`, p.Type), "remotePort = 19000"}, expect...)})
	}
	plugin("socks5", &v1.UpstreamPlugin{Type: "socks5", Username: sref("p-user"), Password: sref("p-pass")}, `plugin.username = "PLUGIN-USER-VAL"`, `plugin.password = "PLUGIN-PASS-VAL"`)
	plugin("http_proxy", &v1.UpstreamPlugin{Type: "http_proxy", Username: sref("p-user"), Password: sref("p-pass")}, `plugin.httpUser = "PLUGIN-USER-VAL"`, `plugin.httpPassword = "PLUGIN-PASS-VAL"`)
	plugin("http_proxy_httpuser", &v1.UpstreamPlugin{Type: "http_proxy", HTTPUser: sref("h-user"), HTTPPassword: sref("h-pass")}, `plugin.httpUser = "HTTP-USER-VAL"`, `plugin.httpPassword = "HTTP-PASS-VAL"`)
	plugin("static_file", &v1.UpstreamPlugin{Type: "static_file", LocalPath: "/data/public", StripPrefix: "download", HTTPUser: sref("h-user"), HTTPPassword: sref("h-pass")},
		`plugin.localPath = "/data/public"`, `plugin.stripPrefix = "download"`, `plugin.httpUser = "HTTP-USER-VAL"`, `plugin.httpPassword = "HTTP-PASS-VAL"`)
	plugin("unix_domain_socket", &v1.UpstreamPlugin{Type: "unix_domain_socket", UnixPath: "/var/run/docker.sock"}, `plugin.unixPath = "/var/run/docker.sock"`)
	for _, pt := range []string{"https2http", "https2https", "http2https", "http2http"} {
		plugin(pt, &v1.UpstreamPlugin{Type: pt, LocalAddr: "127.0.0.1:8443"}, `plugin.localAddr = "127.0.0.1:8443"`)
	}

	// --- UDP ---
	add(featureCase{name: "udp_maximal", upstream: []v1.Upstream{upstream("udp-max", v1.UpstreamSpec{UDP: &v1.UpstreamSpec_UDP{
		Host: "dns.ns.svc", Port: 53, Server: v1.UpstreamSpec_UDP_Server{Port: 5353}, ProxyProtocol: ptr("v2"),
		Transport: &v1.UpstreamSpec_UDP_Transport{UseEncryption: true, UseCompression: true, BandwidthLimit: &v1.UpstreamSpec_UDP_Transport_BandwidthLimit{Enabled: true, Limit: 1, Type: "MB"}},
	}})}, expect: []string{`type = "udp"`, "localPort = 53", "remotePort = 5353", `transport.proxyProtocolVersion = "v2"`, `transport.bandwidthLimit = "1MB"`}})
	add(featureCase{name: "tcp_and_udp_same_server_port", upstream: []v1.Upstream{
		upstream("dns-tcp", v1.UpstreamSpec{TCP: &v1.UpstreamSpec_TCP{Host: "dns.ns.svc", Port: 53, Server: v1.UpstreamSpec_TCP_Server{Port: 53}}}),
		upstream("dns-udp", v1.UpstreamSpec{UDP: &v1.UpstreamSpec_UDP{Host: "dns.ns.svc", Port: 53, Server: v1.UpstreamSpec_UDP_Server{Port: 53}}}),
	}, expect: []string{`name = "dns-tcp"`, `name = "dns-udp"`}})

	// --- STCP / XTCP ---
	add(featureCase{name: "stcp_maximal", upstream: []v1.Upstream{upstream("stcp-max", v1.UpstreamSpec{STCP: &v1.UpstreamSpec_STCP{
		Host: "svc", Port: 22, SecretKey: v1.UpstreamSpec_STCP_SecretKey{Secret: sec("stcp-key")},
		ProxyProtocol: ptr("v1"), HealthCheck: healthCheck(), Transport: proxyTransport(), AllowUsers: []string{"alice", "bob"},
	}})}, expect: append(append([]string{`type = "stcp"`, `secretKey = "STCP-KEY-VAL"`, `allowUsers = ["alice", "bob"]`}, healthCheckExpect...), proxyTransportExpect...)})
	add(featureCase{name: "xtcp_maximal", upstream: []v1.Upstream{upstream("xtcp-max", v1.UpstreamSpec{XTCP: &v1.UpstreamSpec_XTCP{
		Host: "svc", Port: 22, SecretKey: v1.UpstreamSpec_XTCP_SecretKey{Secret: sec("xtcp-key")},
		ProxyProtocol: ptr("v2"), HealthCheck: healthCheck(), Transport: proxyTransport(), AllowUsers: []string{"*"},
	}})}, expect: append(append([]string{`type = "xtcp"`, `secretKey = "XTCP-KEY-VAL"`, `allowUsers = ["*"]`}, healthCheckExpect...), proxyTransportExpect...)})

	// --- HTTP / HTTPS / TCPMUX ---
	add(featureCase{name: "http_maximal", upstream: []v1.Upstream{upstream("http-max", v1.UpstreamSpec{HTTP: &v1.UpstreamSpec_HTTP{
		Host: "web.ns.svc", Port: 80, Subdomain: "app", CustomDomains: []string{"a.example.com", "b.example.com"}, Locations: []string{"/", "/api"},
		HostHeaderRewrite: "internal.local",
		RequestHeaders:    &v1.HTTPHeaders{Set: map[string]string{"x-from-where": "frp", "X-Real.IP": "1.2.3.4"}},
		ResponseHeaders:   &v1.HTTPHeaders{Set: map[string]string{"x-resp": "yes"}},
		HTTPUser:          sref("h-user"), HTTPPassword: sref("h-pass"),
		HealthCheck: &v1.UpstreamSpec_HTTP_HealthCheck{Type: "http", Path: "/healthz", TimeoutSeconds: 3, IntervalSeconds: 10, MaxFailed: 3},
		Transport:   proxyTransport(), LoadBalancer: &v1.LoadBalancer{Group: "web", GroupKey: sref("groupkey")},
	}})}, expect: append([]string{`type = "http"`, `subdomain = "app"`, `customDomains = ["a.example.com", "b.example.com"]`, `locations = ["/", "/api"]`,
		`hostHeaderRewrite = "internal.local"`, `requestHeaders.set."x-from-where" = "frp"`, `requestHeaders.set."X-Real.IP" = "1.2.3.4"`, `responseHeaders.set."x-resp" = "yes"`,
		`httpUser = "HTTP-USER-VAL"`, `healthCheck.type = "http"`, `healthCheck.path = "/healthz"`, `loadBalancer.group = "web"`}, proxyTransportExpect...)})
	add(featureCase{name: "https_maximal", upstream: []v1.Upstream{upstream("https-max", v1.UpstreamSpec{HTTPS: &v1.UpstreamSpec_HTTPS{
		Host: "web.ns.svc", Port: 443, CustomDomains: []string{"secure.example.com"}, ProxyProtocol: ptr("v2"), Transport: proxyTransport(),
		LoadBalancer: &v1.LoadBalancer{Group: "web", GroupKey: sref("groupkey")},
	}})}, expect: append([]string{`type = "https"`, `customDomains = ["secure.example.com"]`, `transport.proxyProtocolVersion = "v2"`, `loadBalancer.group = "web"`}, proxyTransportExpect...)})
	add(featureCase{name: "tcpmux_maximal", upstream: []v1.Upstream{upstream("mux-max", v1.UpstreamSpec{TCPMUX: &v1.UpstreamSpec_TCPMUX{
		Host: "svc", Port: 22, Multiplexer: "httpconnect", CustomDomains: []string{"mux.example.com"}, Transport: proxyTransport(),
	}})}, expect: append([]string{`type = "tcpmux"`, `multiplexer = "httpconnect"`, `customDomains = ["mux.example.com"]`}, proxyTransportExpect...)})

	// --- Visitors ---
	add(featureCase{name: "visitor_stcp", visitor: []v1.Visitor{visitor("v-stcp", v1.VisitorSpec{STCP: &v1.VisitorSpec_STCP{
		Host: "0.0.0.0", Port: 6000, ServerName: "stcp-max", ServerSecretKey: v1.VisitorSpec_STCP_ServerSecretKey{Secret: sec("stcp-key")},
	}})}, expect: []string{"[[visitors]]", `type = "stcp"`, `serverName = "stcp-max"`, `secretKey = "STCP-KEY-VAL"`, `bindAddr = "0.0.0.0"`, "bindPort = 6000"}})
	add(featureCase{name: "visitor_xtcp_assisted", visitor: []v1.Visitor{visitor("v-xtcp", v1.VisitorSpec{XTCP: &v1.VisitorSpec_XTCP{
		Host: "0.0.0.0", Port: 6001, ServerName: "xtcp-max", ServerSecretKey: v1.VisitorSpec_XTCP_ServerSecretKey{Secret: sec("xtcp-key")},
		PersistantConnection: true, EnableAssistedAddrs: true,
	}})}, expect: []string{`type = "xtcp"`, "keepTunnelOpen = true", "bindPort = 6001"}})
	add(featureCase{name: "visitor_xtcp_default_fallback", visitor: []v1.Visitor{visitor("v-xtcp", v1.VisitorSpec{XTCP: &v1.VisitorSpec_XTCP{
		Host: "0.0.0.0", Port: 6001, ServerName: "xtcp-max", ServerSecretKey: v1.VisitorSpec_XTCP_ServerSecretKey{Secret: sec("xtcp-key")},
		Fallback: &v1.VisitorSpec_Fallback{ServerName: "stcp-max", Timeout: 200},
	}})}, expect: []string{"natTraversal.disableAssistedAddrs = true", `fallbackTo = "v-xtcp-fallback"`, "fallbackTimeoutMs = 200", `name = "v-xtcp-fallback"`, "bindPort = -1"}})
	c = baseClient()
	c.Spec.User = "bob"
	add(featureCase{name: "visitor_cross_user", client: c, visitor: []v1.Visitor{
		visitor("v-stcp", v1.VisitorSpec{STCP: &v1.VisitorSpec_STCP{
			Host: "0.0.0.0", Port: 6000, ServerUser: "alice", ServerName: "stcp-max", ServerSecretKey: v1.VisitorSpec_STCP_ServerSecretKey{Secret: sec("stcp-key")},
		}}),
		visitor("v-xtcp", v1.VisitorSpec{XTCP: &v1.VisitorSpec_XTCP{
			Host: "0.0.0.0", Port: 6001, ServerUser: "alice", ServerName: "xtcp-max", ServerSecretKey: v1.VisitorSpec_XTCP_ServerSecretKey{Secret: sec("xtcp-key")},
			Fallback: &v1.VisitorSpec_Fallback{ServerName: "stcp-max", Timeout: 200},
		}}),
	}, expect: []string{`user = "bob"`, `serverUser = "alice"`}})
	add(featureCase{name: "visitor_xtcp_fallback_secret", visitor: []v1.Visitor{visitor("v-xtcp", v1.VisitorSpec{XTCP: &v1.VisitorSpec_XTCP{
		Host: "0.0.0.0", Port: 6001, ServerName: "xtcp-max", ServerSecretKey: v1.VisitorSpec_XTCP_ServerSecretKey{Secret: sec("xtcp-key")},
		Fallback: &v1.VisitorSpec_Fallback{ServerName: "stcp-max", Timeout: 200, ServerSecretKey: sref("stcp-key")},
	}})}, expect: []string{`secretKey = "XTCP-KEY-VAL"`, `secretKey = "STCP-KEY-VAL"`}})
	add(featureCase{name: "visitor_disabled", visitor: []v1.Visitor{visitor("v-off", v1.VisitorSpec{Enabled: ptr(false), STCP: &v1.VisitorSpec_STCP{
		Host: "0.0.0.0", Port: 6000, ServerName: "s", ServerSecretKey: v1.VisitorSpec_STCP_ServerSecretKey{Secret: sec("stcp-key")},
	}})}, expect: []string{"enabled = false"}})

	return cs
}

func renderFeatureCases(t *testing.T) []rendered {
	var out []rendered
	for _, fc := range featureCases() {
		config, err := render(t, []client.Object{credsSecret()}, fc.client, fc.upstream, fc.visitor)
		if err != nil {
			t.Errorf("%s: %v", fc.name, err)
			continue
		}
		out = append(out, rendered{name: fc.name, config: config, expect: fc.expect})
	}
	return out
}

// ---------------------------------------------------------------------------------------
// Examples: every YAML under examples/ is decoded strictly and each Client is rendered.
// ---------------------------------------------------------------------------------------

type exampleSet struct {
	clients   map[string]*v1.Client
	upstreams []v1.Upstream
	visitors  []v1.Visitor
	objects   []client.Object
	pools     []*v1.ServerPool
	services  []*corev1.Service
}

var docSeparator = regexp.MustCompile(`(?m)^---\s*$`)

// exampleKey groups files by example directory; the client/ and deployment/ subdirectories
// belong to their parent (e.g. examples/simple/client/*.yaml → "simple").
func exampleKey(rel string) string {
	dir := filepath.Dir(rel)
	for base := filepath.Base(dir); base == "client" || base == "deployment"; base = filepath.Base(dir) {
		dir = filepath.Dir(dir)
	}
	return dir
}

func loadExamples(t *testing.T) map[string]*exampleSet {
	t.Helper()
	root := filepath.Join("..", "..", "examples")
	sets := map[string]*exampleSet{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		set := sets[exampleKey(rel)]
		if set == nil {
			set = &exampleSet{clients: map[string]*v1.Client{}}
			sets[exampleKey(rel)] = set
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, doc := range docSeparator.Split(string(data), -1) {
			var meta metav1.TypeMeta
			if err := yaml.Unmarshal([]byte(doc), &meta); err != nil || meta.Kind == "" {
				continue
			}
			decode := func(obj client.Object) {
				if err := yaml.UnmarshalStrict([]byte(doc), obj); err != nil {
					t.Errorf("examples/%s: %s does not decode strictly: %v", rel, meta.Kind, err)
				}
				if obj.GetNamespace() == "" {
					obj.SetNamespace(ns)
				}
			}
			switch meta.Kind {
			case "Client":
				o := &v1.Client{}
				decode(o)
				set.clients[o.Name] = o
			case "Upstream":
				o := &v1.Upstream{}
				decode(o)
				set.upstreams = append(set.upstreams, *o)
			case "Visitor":
				o := &v1.Visitor{}
				decode(o)
				set.visitors = append(set.visitors, *o)
			case "Secret":
				o := &corev1.Secret{}
				decode(o)
				for k, v := range o.StringData {
					if o.Data == nil {
						o.Data = map[string][]byte{}
					}
					o.Data[k] = []byte(v)
				}
				set.objects = append(set.objects, o)
			case "ServerPool":
				o := &v1.ServerPool{}
				decode(o)
				set.pools = append(set.pools, o)
			case "Service":
				o := &corev1.Service{}
				decode(o)
				set.services = append(set.services, o)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sets
}

func renderExamples(t *testing.T, sets map[string]*exampleSet) []rendered {
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []rendered
	for _, key := range keys {
		set := sets[key]
		for _, u := range set.upstreams {
			if _, ok := set.clients[u.Spec.Client]; !ok {
				t.Errorf("examples/%s: Upstream %q references Client %q, which the example does not define", key, u.Name, u.Spec.Client)
			}
		}
		for _, v := range set.visitors {
			if _, ok := set.clients[v.Spec.Client]; !ok {
				t.Errorf("examples/%s: Visitor %q references Client %q, which the example does not define", key, v.Name, v.Spec.Client)
			}
		}

		names := make([]string, 0, len(set.clients))
		for n := range set.clients {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			var ups []v1.Upstream
			var vis []v1.Visitor
			for _, u := range set.upstreams {
				if u.Spec.Client == n {
					ups = append(ups, u)
				}
			}
			for _, v := range set.visitors {
				if v.Spec.Client == n {
					vis = append(vis, v)
				}
			}
			name := "example_" + strings.ReplaceAll(key, string(filepath.Separator), "_") + "_" + n
			config, err := render(t, set.objects, set.clients[n], ups, vis)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			out = append(out, rendered{name: name, config: config})
		}
	}
	return out
}

// ---------------------------------------------------------------------------------------
// ServerPool: the LoadBalancer path builds Client/Upstream CRs from a pool + Service.
// ---------------------------------------------------------------------------------------

func renderServerPool(t *testing.T, set *exampleSet) []rendered {
	if set == nil || len(set.pools) == 0 {
		t.Fatal("examples/loadbalancer must define a ServerPool")
	}
	dns := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: ns, UID: "uid-dns"},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{
			{Name: "dns-udp", Port: 53, Protocol: corev1.ProtocolUDP},
			{Name: "dns-tcp", Port: 53, Protocol: corev1.ProtocolTCP},
		}},
	}
	services := append(append([]*corev1.Service{}, set.services...), dns)

	variants := map[string]func(*v1.ServerPoolServer){
		"example": func(*v1.ServerPoolServer) {},
		"maximal": func(s *v1.ServerPoolServer) {
			s.TransportProtocol = ptr("quic")
			s.TLS = &v1.ClientSpec_Server_TLS{Enable: true}
			s.Transport = &v1.ClientSpec_Server_Transport{PoolCount: 2, TCPMux: ptr(false), DialServerTimeout: "10s", WireProtocol: "v2"}
		},
	}

	var out []rendered
	for variant, mutate := range variants {
		pool := set.pools[0].DeepCopy()
		for i := range pool.Spec.Servers {
			mutate(&pool.Spec.Servers[i])
		}
		server := pool.Spec.Servers[0]
		ref := loadbalancer.ServerRef{Pool: pool.Name, Server: server.Name}
		for _, svc := range services {
			c := loadbalancer.BuildClient(pool, server, svc, ref)
			var ups []v1.Upstream
			for _, port := range svc.Spec.Ports {
				ups = append(ups, *loadbalancer.BuildUpstream(pool, svc, port, ref))
			}
			name := fmt.Sprintf("serverpool_%s_%s", variant, svc.Name)
			config, err := render(t, set.objects, c, ups, nil)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			var expect []string
			if server.TransportProtocol != nil {
				expect = append(expect, fmt.Sprintf(`transport.protocol = "%s"`, *server.TransportProtocol))
			}
			out = append(out, rendered{name: name, config: config, expect: expect})
		}
	}
	return out
}
