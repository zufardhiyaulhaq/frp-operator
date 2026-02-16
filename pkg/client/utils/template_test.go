package utils

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

// Test data structures that mirror models package
type testConfig struct {
	Common    testCommon
	Upstreams []testUpstream
	Visitors  []testVisitor
}

type testCommon struct {
	ServerAddress        string
	ServerPort           int
	ServerProtocol       string
	ServerAuthentication testServerAuthentication
	AdminAddress         string
	AdminPort            int
	AdminUsername        string
	AdminPassword        string
	STUNServer           *string
}

type testServerAuthentication struct {
	Type  int
	Token string
}

type testUpstream struct {
	Name  string
	Type  int
	TCP   testUpstreamTCP
	UDP   testUpstreamUDP
	STCP  testUpstreamSTCP
	XTCP  testUpstreamSTCP
	HTTP  testUpstreamHTTP
	HTTPS testUpstreamHTTPS
}

type testUpstreamTCP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	HealthCheck   *testHealthCheck
	Transport     *testTransport
}

type testUpstreamUDP struct {
	Host       string
	Port       int
	ServerPort int
}

type testUpstreamSTCP struct {
	Host          string
	Port          int
	SecretKey     string
	ProxyProtocol *string
	HealthCheck   *testHealthCheck
	Transport     *testTransport
}

type testUpstreamHTTP struct {
	Host              string
	Port              int
	Subdomain         string
	CustomDomains     []string
	Locations         []string
	HostHeaderRewrite string
	RequestHeaders    map[string]string
	ResponseHeaders   map[string]string
	HTTPUser          string
	HTTPPassword      string
	HealthCheck       *testHTTPHealthCheck
	Transport         *testTransport
}

type testUpstreamHTTPS struct {
	Host          string
	Port          int
	CustomDomains []string
	ProxyProtocol *string
	Transport     *testTransport
}

type testHealthCheck struct {
	TimeoutSeconds  int
	MaxFailed       int
	IntervalSeconds int
}

type testHTTPHealthCheck struct {
	Type            string
	Path            string
	TimeoutSeconds  int
	MaxFailed       int
	IntervalSeconds int
}

type testTransport struct {
	UseCompression bool
	UseEncryption  bool
	BandwdithLimit *testBandwidthLimit
	ProxyURL       *string
}

type testBandwidthLimit struct {
	Enabled bool
	Limit   int
	Type    string
}

type testVisitor struct {
	Name string
	Type int
	STCP testVisitorSTCP
	XTCP testVisitorXTCP
}

type testVisitorSTCP struct {
	Host       string
	Port       int
	ServerName string
	SecretKey  string
}

type testVisitorXTCP struct {
	Host                 string
	Port                 int
	ServerName           string
	SecretKey            string
	PersistantConnection bool
	EnableAssistedAddrs  bool
	Fallback             *testVisitorFallback
}

type testVisitorFallback struct {
	ServerName string
	Timeout    int
}

// Helper function to render template
func renderTemplate(t *testing.T, config testConfig) string {
	t.Helper()
	var buf bytes.Buffer
	tmpl, err := template.New("frpc").Parse(CLIENT_TEMPLATE)
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}
	err = tmpl.Execute(&buf, config)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}
	return buf.String()
}

// Helper to check if output contains expected strings
func assertContains(t *testing.T, output, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
	}
}

// Helper to check if output does NOT contain a string
func assertNotContains(t *testing.T, output, unexpected string) {
	t.Helper()
	if strings.Contains(output, unexpected) {
		t.Errorf("Expected output NOT to contain %q, but it did.\nOutput:\n%s", unexpected, output)
	}
}

func TestTemplateParseValid(t *testing.T) {
	_, err := template.New("frpc").Parse(CLIENT_TEMPLATE)
	if err != nil {
		t.Fatalf("CLIENT_TEMPLATE should parse without error: %v", err)
	}
}

func TestTemplateCommonSection(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress:  "frp.example.com",
			ServerPort:     7000,
			AdminAddress:   "0.0.0.0",
			AdminPort:      7400,
			AdminUsername:  "admin",
			AdminPassword:  "secret",
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `serverAddr = "frp.example.com"`)
	assertContains(t, output, `serverPort = 7000`)
	assertContains(t, output, `webServer.addr = "0.0.0.0"`)
	assertContains(t, output, `webServer.port = 7400`)
	assertContains(t, output, `webServer.user = "admin"`)
	assertContains(t, output, `webServer.password = "secret"`)
}

func TestTemplateCommonWithTokenAuth(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			ServerAuthentication: testServerAuthentication{
				Type:  1,
				Token: "my-secret-token",
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `auth.method = "token"`)
	assertContains(t, output, `auth.token = "my-secret-token"`)
}

func TestTemplateCommonWithSTUNServer(t *testing.T) {
	stunServer := "stun.example.com:3478"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			STUNServer:    &stunServer,
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `natHoleStunServer = "stun.example.com:3478"`)
}

func TestTemplateCommonWithoutSTUNServer(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			STUNServer:    nil,
		},
	}

	output := renderTemplate(t, config)

	assertNotContains(t, output, `natHoleStunServer`)
}

func TestTemplateTCPUpstreamBasic(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:       "localhost",
					Port:       8080,
					ServerPort: 9080,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "tcp-service"`)
	assertContains(t, output, `type = "tcp"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 8080`)
	assertContains(t, output, `remotePort = 9080`)
}

func TestTemplateTCPUpstreamWithProxyProtocol(t *testing.T) {
	proxyProtocol := "v2"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:          "localhost",
					Port:          8080,
					ServerPort:    9080,
					ProxyProtocol: &proxyProtocol,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.proxyProtocolVersion = "v2"`)
}

func TestTemplateTCPUpstreamWithHealthCheck(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:       "localhost",
					Port:       8080,
					ServerPort: 9080,
					HealthCheck: &testHealthCheck{
						TimeoutSeconds:  3,
						MaxFailed:       5,
						IntervalSeconds: 10,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `healthCheck.type = "tcp"`)
	assertContains(t, output, `healthCheck.timeoutSeconds = 3`)
	assertContains(t, output, `healthCheck.maxFailed = 5`)
	assertContains(t, output, `healthCheck.intervalSeconds = 10`)
}

func TestTemplateTCPUpstreamWithTransport(t *testing.T) {
	proxyURL := "http://proxy.example.com:8080"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:       "localhost",
					Port:       8080,
					ServerPort: 9080,
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: true,
						BandwdithLimit: &testBandwidthLimit{
							Enabled: true,
							Limit:   10,
							Type:    "MB",
						},
						ProxyURL: &proxyURL,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.useEncryption = true`)
	assertContains(t, output, `transport.useCompression = true`)
	assertContains(t, output, `transport.bandwidthLimit = "10MB"`)
	assertContains(t, output, `transport.bandwidthLimitMode = "client"`)
	assertContains(t, output, `transport.proxyURL = "http://proxy.example.com:8080"`)
}

func TestTemplateUDPUpstream(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "udp-service",
				Type: 2,
				UDP: testUpstreamUDP{
					Host:       "localhost",
					Port:       53,
					ServerPort: 5353,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "udp-service"`)
	assertContains(t, output, `type = "udp"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 53`)
	assertContains(t, output, `remotePort = 5353`)
}

func TestTemplateSTCPUpstream(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "stcp-service",
				Type: 3,
				STCP: testUpstreamSTCP{
					Host:      "localhost",
					Port:      22,
					SecretKey: "my-stcp-secret",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "stcp-service"`)
	assertContains(t, output, `type = "stcp"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 22`)
	assertContains(t, output, `secretKey = "my-stcp-secret"`)
}

func TestTemplateSTCPUpstreamWithAllOptions(t *testing.T) {
	proxyProtocol := "v1"
	proxyURL := "socks5://proxy:1080"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "stcp-service",
				Type: 3,
				STCP: testUpstreamSTCP{
					Host:          "localhost",
					Port:          22,
					SecretKey:     "my-stcp-secret",
					ProxyProtocol: &proxyProtocol,
					HealthCheck: &testHealthCheck{
						TimeoutSeconds:  5,
						MaxFailed:       3,
						IntervalSeconds: 30,
					},
					Transport: &testTransport{
						UseEncryption:  false,
						UseCompression: true,
						BandwdithLimit: &testBandwidthLimit{
							Enabled: true,
							Limit:   5,
							Type:    "KB",
						},
						ProxyURL: &proxyURL,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `type = "stcp"`)
	assertContains(t, output, `secretKey = "my-stcp-secret"`)
	assertContains(t, output, `transport.proxyProtocolVersion = "v1"`)
	assertContains(t, output, `healthCheck.type = "tcp"`)
	assertContains(t, output, `healthCheck.timeoutSeconds = 5`)
	assertContains(t, output, `transport.useEncryption = false`)
	assertContains(t, output, `transport.useCompression = true`)
	assertContains(t, output, `transport.bandwidthLimit = "5KB"`)
	assertContains(t, output, `transport.proxyURL = "socks5://proxy:1080"`)
}

func TestTemplateXTCPUpstream(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "xtcp-service",
				Type: 4,
				XTCP: testUpstreamSTCP{
					Host:      "localhost",
					Port:      3389,
					SecretKey: "my-xtcp-secret",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "xtcp-service"`)
	assertContains(t, output, `type = "xtcp"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 3389`)
	assertContains(t, output, `secretKey = "my-xtcp-secret"`)
}

func TestTemplateHTTPUpstreamBasic(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host: "localhost",
					Port: 80,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "http-service"`)
	assertContains(t, output, `type = "http"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 80`)
}

func TestTemplateHTTPUpstreamWithSubdomain(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:      "localhost",
					Port:      80,
					Subdomain: "myapp",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `subdomain = "myapp"`)
}

func TestTemplateHTTPUpstreamWithCustomDomains(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:          "localhost",
					Port:          80,
					CustomDomains: []string{"example.com", "www.example.com"},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `customDomains = ["example.com", "www.example.com"]`)
}

func TestTemplateHTTPUpstreamWithLocations(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:      "localhost",
					Port:      80,
					Locations: []string{"/api", "/web"},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `locations = ["/api", "/web"]`)
}

func TestTemplateHTTPUpstreamWithHostHeaderRewrite(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:              "localhost",
					Port:              80,
					HostHeaderRewrite: "internal.example.com",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `hostHeaderRewrite = "internal.example.com"`)
}

func TestTemplateHTTPUpstreamWithRequestHeaders(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host: "localhost",
					Port: 80,
					RequestHeaders: map[string]string{
						"X-Custom-Header": "custom-value",
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `requestHeaders.set.X-Custom-Header = "custom-value"`)
}

func TestTemplateHTTPUpstreamWithResponseHeaders(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host: "localhost",
					Port: 80,
					ResponseHeaders: map[string]string{
						"X-Server": "frp-operator",
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `responseHeaders.set.X-Server = "frp-operator"`)
}

func TestTemplateHTTPUpstreamWithAuth(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:         "localhost",
					Port:         80,
					HTTPUser:     "user123",
					HTTPPassword: "pass456",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `httpUser = "user123"`)
	assertContains(t, output, `httpPassword = "pass456"`)
}

func TestTemplateHTTPUpstreamWithHealthCheck(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host: "localhost",
					Port: 80,
					HealthCheck: &testHTTPHealthCheck{
						Type:            "http",
						Path:            "/health",
						TimeoutSeconds:  5,
						MaxFailed:       3,
						IntervalSeconds: 10,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `healthCheck.type = "http"`)
	assertContains(t, output, `healthCheck.path = "/health"`)
	assertContains(t, output, `healthCheck.timeoutSeconds = 5`)
	assertContains(t, output, `healthCheck.maxFailed = 3`)
	assertContains(t, output, `healthCheck.intervalSeconds = 10`)
}

func TestTemplateHTTPUpstreamWithTransport(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host: "localhost",
					Port: 80,
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: true,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.useEncryption = true`)
	assertContains(t, output, `transport.useCompression = true`)
}

func TestTemplateHTTPSUpstreamBasic(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "https-service",
				Type: 6,
				HTTPS: testUpstreamHTTPS{
					Host:          "localhost",
					Port:          443,
					CustomDomains: []string{"secure.example.com"},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[proxies]]`)
	assertContains(t, output, `name = "https-service"`)
	assertContains(t, output, `type = "https"`)
	assertContains(t, output, `localIP = "localhost"`)
	assertContains(t, output, `localPort = 443`)
	assertContains(t, output, `customDomains = ["secure.example.com"]`)
}

func TestTemplateHTTPSUpstreamWithProxyProtocol(t *testing.T) {
	proxyProtocol := "v2"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "https-service",
				Type: 6,
				HTTPS: testUpstreamHTTPS{
					Host:          "localhost",
					Port:          443,
					CustomDomains: []string{"secure.example.com"},
					ProxyProtocol: &proxyProtocol,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.proxyProtocolVersion = "v2"`)
}

func TestTemplateHTTPSUpstreamWithTransport(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "https-service",
				Type: 6,
				HTTPS: testUpstreamHTTPS{
					Host:          "localhost",
					Port:          443,
					CustomDomains: []string{"secure.example.com"},
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: false,
						BandwdithLimit: &testBandwidthLimit{
							Enabled: true,
							Limit:   100,
							Type:    "MB",
						},
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.useEncryption = true`)
	assertContains(t, output, `transport.useCompression = false`)
	assertContains(t, output, `transport.bandwidthLimit = "100MB"`)
}

func TestTemplateHTTPSUpstreamWithProxyURL(t *testing.T) {
	proxyURL := "http://proxy.example.com:8080"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "https-service",
				Type: 6,
				HTTPS: testUpstreamHTTPS{
					Host:          "localhost",
					Port:          443,
					CustomDomains: []string{"secure.example.com"},
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: true,
						ProxyURL:       &proxyURL,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `transport.proxyURL = "http://proxy.example.com:8080"`)
}

func TestTemplateSTCPVisitor(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Visitors: []testVisitor{
			{
				Name: "stcp-visitor",
				Type: 1,
				STCP: testVisitorSTCP{
					Host:       "127.0.0.1",
					Port:       6000,
					ServerName: "remote-stcp",
					SecretKey:  "shared-secret",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[visitors]]`)
	assertContains(t, output, `name = "stcp-visitor"`)
	assertContains(t, output, `type = "stcp"`)
	assertContains(t, output, `serverName = "remote-stcp"`)
	assertContains(t, output, `secretKey = "shared-secret"`)
	assertContains(t, output, `bindAddr = "127.0.0.1"`)
	assertContains(t, output, `bindPort = 6000`)
}

func TestTemplateXTCPVisitor(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Visitors: []testVisitor{
			{
				Name: "xtcp-visitor",
				Type: 2,
				XTCP: testVisitorXTCP{
					Host:                 "127.0.0.1",
					Port:                 7000,
					ServerName:           "remote-xtcp",
					SecretKey:            "xtcp-secret",
					PersistantConnection: true,
					EnableAssistedAddrs:  false,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `[[visitors]]`)
	assertContains(t, output, `name = "xtcp-visitor"`)
	assertContains(t, output, `type = "xtcp"`)
	assertContains(t, output, `serverName = "remote-xtcp"`)
	assertContains(t, output, `secretKey = "xtcp-secret"`)
	assertContains(t, output, `bindAddr = "127.0.0.1"`)
	assertContains(t, output, `bindPort = 7000`)
	assertContains(t, output, `keepTunnelOpen = true`)
	assertContains(t, output, `natHoleStun.disableAssistedAddrs = true`)
}

func TestTemplateXTCPVisitorWithAssistedAddrs(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Visitors: []testVisitor{
			{
				Name: "xtcp-visitor",
				Type: 2,
				XTCP: testVisitorXTCP{
					Host:                 "127.0.0.1",
					Port:                 7000,
					ServerName:           "remote-xtcp",
					SecretKey:            "xtcp-secret",
					PersistantConnection: false,
					EnableAssistedAddrs:  true,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `keepTunnelOpen = false`)
	assertNotContains(t, output, `natHoleStun.disableAssistedAddrs`)
}

func TestTemplateXTCPVisitorWithFallback(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Visitors: []testVisitor{
			{
				Name: "xtcp-visitor",
				Type: 2,
				XTCP: testVisitorXTCP{
					Host:                 "127.0.0.1",
					Port:                 7000,
					ServerName:           "remote-xtcp",
					SecretKey:            "xtcp-secret",
					PersistantConnection: true,
					Fallback: &testVisitorFallback{
						ServerName: "fallback-stcp",
						Timeout:    5000,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	assertContains(t, output, `fallbackTo = "xtcp-visitor-fallback"`)
	assertContains(t, output, `fallbackTimeoutMs = 5000`)
	// Check the fallback visitor is created
	assertContains(t, output, `name = "xtcp-visitor-fallback"`)
	assertContains(t, output, `serverName = "fallback-stcp"`)
	assertContains(t, output, `bindPort = -1`)
}

func TestTemplateMultipleUpstreams(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:       "localhost",
					Port:       8080,
					ServerPort: 9080,
				},
			},
			{
				Name: "http-service",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:      "localhost",
					Port:      80,
					Subdomain: "web",
				},
			},
		},
	}

	output := renderTemplate(t, config)

	// Count [[proxies]] occurrences
	count := strings.Count(output, "[[proxies]]")
	if count != 2 {
		t.Errorf("Expected 2 [[proxies]] sections, got %d", count)
	}

	assertContains(t, output, `name = "tcp-service"`)
	assertContains(t, output, `name = "http-service"`)
}

func TestTemplateMultipleVisitors(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Visitors: []testVisitor{
			{
				Name: "stcp-visitor",
				Type: 1,
				STCP: testVisitorSTCP{
					Host:       "127.0.0.1",
					Port:       6000,
					ServerName: "remote-stcp",
					SecretKey:  "secret1",
				},
			},
			{
				Name: "xtcp-visitor",
				Type: 2,
				XTCP: testVisitorXTCP{
					Host:                 "127.0.0.1",
					Port:                 7000,
					ServerName:           "remote-xtcp",
					SecretKey:            "secret2",
					PersistantConnection: true,
				},
			},
		},
	}

	output := renderTemplate(t, config)

	// Count [[visitors]] occurrences
	count := strings.Count(output, "[[visitors]]")
	if count != 2 {
		t.Errorf("Expected 2 [[visitors]] sections, got %d", count)
	}

	assertContains(t, output, `name = "stcp-visitor"`)
	assertContains(t, output, `name = "xtcp-visitor"`)
}

func TestTemplateEmptyUpstreamsAndVisitors(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{},
		Visitors:  []testVisitor{},
	}

	output := renderTemplate(t, config)

	// Should still have common section
	assertContains(t, output, `serverAddr = "frp.example.com"`)
	// Should not have any proxies or visitors
	assertNotContains(t, output, `[[proxies]]`)
	assertNotContains(t, output, `[[visitors]]`)
}

func TestTemplateHTTPUpstreamFull(t *testing.T) {
	proxyURL := "http://proxy:8080"
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "full-http",
				Type: 5,
				HTTP: testUpstreamHTTP{
					Host:              "backend.local",
					Port:              8080,
					Subdomain:         "api",
					CustomDomains:     []string{"api.example.com", "api2.example.com"},
					Locations:         []string{"/v1", "/v2"},
					HostHeaderRewrite: "backend.internal",
					RequestHeaders: map[string]string{
						"X-Forwarded-Proto": "https",
					},
					ResponseHeaders: map[string]string{
						"X-Powered-By": "frp",
					},
					HTTPUser:     "apiuser",
					HTTPPassword: "apipass",
					HealthCheck: &testHTTPHealthCheck{
						Type:            "http",
						Path:            "/healthz",
						TimeoutSeconds:  10,
						MaxFailed:       5,
						IntervalSeconds: 30,
					},
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: true,
						BandwdithLimit: &testBandwidthLimit{
							Enabled: true,
							Limit:   50,
							Type:    "MB",
						},
						ProxyURL: &proxyURL,
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	// Verify all HTTP features
	assertContains(t, output, `type = "http"`)
	assertContains(t, output, `localIP = "backend.local"`)
	assertContains(t, output, `localPort = 8080`)
	assertContains(t, output, `subdomain = "api"`)
	assertContains(t, output, `customDomains = ["api.example.com", "api2.example.com"]`)
	assertContains(t, output, `locations = ["/v1", "/v2"]`)
	assertContains(t, output, `hostHeaderRewrite = "backend.internal"`)
	assertContains(t, output, `requestHeaders.set.X-Forwarded-Proto = "https"`)
	assertContains(t, output, `responseHeaders.set.X-Powered-By = "frp"`)
	assertContains(t, output, `httpUser = "apiuser"`)
	assertContains(t, output, `httpPassword = "apipass"`)
	assertContains(t, output, `healthCheck.type = "http"`)
	assertContains(t, output, `healthCheck.path = "/healthz"`)
	assertContains(t, output, `transport.useEncryption = true`)
	assertContains(t, output, `transport.bandwidthLimit = "50MB"`)
	assertContains(t, output, `transport.proxyURL = "http://proxy:8080"`)
}

func TestTemplateBandwidthLimitDisabled(t *testing.T) {
	config := testConfig{
		Common: testCommon{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
		Upstreams: []testUpstream{
			{
				Name: "tcp-service",
				Type: 1,
				TCP: testUpstreamTCP{
					Host:       "localhost",
					Port:       8080,
					ServerPort: 9080,
					Transport: &testTransport{
						UseEncryption:  true,
						UseCompression: false,
						BandwdithLimit: &testBandwidthLimit{
							Enabled: false,
							Limit:   10,
							Type:    "MB",
						},
					},
				},
			},
		},
	}

	output := renderTemplate(t, config)

	// Should have transport settings but NOT bandwidth limit
	assertContains(t, output, `transport.useEncryption = true`)
	assertNotContains(t, output, `transport.bandwidthLimit`)
	assertNotContains(t, output, `transport.bandwidthLimitMode`)
}
