# Phase 4: Advanced Features Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add load balancing, FRP plugins, TCPMUX proxy type, and client transport tuning for complex deployments.

**Architecture:** Extend upstream types with LoadBalancer and Plugin fields, add TCPMUX upstream type, add Transport configuration to Client, update templates and models.

**Tech Stack:** Go, Kubebuilder v3, controller-runtime, text/template

---

## Task 1: Add Load Balancer Types

**Files:**
- Modify: `api/v1alpha1/upstream_types.go`

**Step 1: Add LoadBalancer type**

Add to `api/v1alpha1/upstream_types.go`:

```go
type LoadBalancer struct {
	// Group is the load balancer group name
	Group string `json:"group"`
	// +optional
	// GroupKey is the shared secret for the group
	GroupKey *SecretRef `json:"groupKey,omitempty"`
}
```

**Step 2: Add LoadBalancer to TCP upstream**

Modify `UpstreamSpec_TCP`:

```go
type UpstreamSpec_TCP struct {
	Host   string                  `json:"host"`
	Port   int                     `json:"port"`
	Server UpstreamSpec_TCP_Server `json:"server"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport"`
	// +optional
	LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/upstream_types.go config/crd/
git commit -m "feat(api): add LoadBalancer to TCP upstream"
```

---

## Task 2: Add Load Balancer to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Add LoadBalancer to Upstream_TCP model**

```go
type Upstream_TCP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	LoadBalancer  *LoadBalancerConfig
}

type LoadBalancerConfig struct {
	Group    string
	GroupKey string
}
```

**Step 2: Write failing test**

```go
{
	name: "TCP upstream - with load balancer",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "tcp-lb-node1",
				Type: 1,
				TCP: models.Upstream_TCP{
					Host:       "api-1.default.svc",
					Port:       8080,
					ServerPort: 9000,
					LoadBalancer: &models.LoadBalancerConfig{
						Group:    "api-cluster",
						GroupKey: "secret-key",
					},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "tcp-lb-node1"`,
		`type = "tcp"`,
		`loadBalancer.group = "api-cluster"`,
		`loadBalancer.groupKey = "secret-key"`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add load balancer to TCP template**

Add in TCP section after transport:

```go
{{ if $upstream.TCP.LoadBalancer }}
loadBalancer.group = "{{ $upstream.TCP.LoadBalancer.Group }}"
{{ if $upstream.TCP.LoadBalancer.GroupKey }}
loadBalancer.groupKey = "{{ $upstream.TCP.LoadBalancer.GroupKey }}"
{{ end }}
{{ end }}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 6: Update NewConfig for load balancer**

```go
if upstreamObject.Spec.TCP.LoadBalancer != nil {
	upstream.TCP.LoadBalancer = &LoadBalancerConfig{
		Group: upstreamObject.Spec.TCP.LoadBalancer.Group,
	}

	if upstreamObject.Spec.TCP.LoadBalancer.GroupKey != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      upstreamObject.Spec.TCP.LoadBalancer.GroupKey.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[upstreamObject.Spec.TCP.LoadBalancer.GroupKey.Secret.Key]; ok {
				upstream.TCP.LoadBalancer.GroupKey = string(val)
			}
		}
	}
}
```

**Step 7: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add load balancer support for TCP upstreams"
```

---

## Task 3: Add Plugin Support Types

**Files:**
- Modify: `api/v1alpha1/upstream_types.go`

**Step 1: Add Plugin type**

```go
type UpstreamPlugin struct {
	// +kubebuilder:validation:Enum=socks5;http_proxy;static_file;https2http;https2https;http2http;http2https;unix_domain_socket
	Type string `json:"type"`

	// For socks5, http_proxy
	// +optional
	Username *SecretRef `json:"username,omitempty"`
	// +optional
	Password *SecretRef `json:"password,omitempty"`

	// For static_file
	// +optional
	LocalPath string `json:"localPath,omitempty"`
	// +optional
	StripPrefix string `json:"stripPrefix,omitempty"`
	// +optional
	HTTPUser *SecretRef `json:"httpUser,omitempty"`
	// +optional
	HTTPPassword *SecretRef `json:"httpPassword,omitempty"`

	// For https2http, https2https, http2https
	// +optional
	LocalAddr string `json:"localAddr,omitempty"`

	// For unix_domain_socket
	// +optional
	UnixPath string `json:"unixPath,omitempty"`
}
```

**Step 2: Add Plugin to TCP upstream**

Make Host/Port optional and add Plugin:

```go
type UpstreamSpec_TCP struct {
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	Port int `json:"port,omitempty"`
	Server UpstreamSpec_TCP_Server `json:"server"`
	// +optional
	Plugin *UpstreamPlugin `json:"plugin,omitempty"`
	// ... rest of fields ...
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/upstream_types.go config/crd/
git commit -m "feat(api): add Plugin support to upstream types"
```

---

## Task 4: Add Plugin to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Add Plugin model**

```go
type Upstream_TCP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	LoadBalancer  *LoadBalancerConfig
	Plugin        *PluginConfig
}

type PluginConfig struct {
	Type         string
	Username     string
	Password     string
	LocalPath    string
	StripPrefix  string
	HTTPUser     string
	HTTPPassword string
	LocalAddr    string
	UnixPath     string
}
```

**Step 2: Write failing test for socks5 plugin**

```go
{
	name: "TCP upstream - socks5 plugin",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "socks5-proxy",
				Type: 1,
				TCP: models.Upstream_TCP{
					ServerPort: 1080,
					Plugin: &models.PluginConfig{
						Type:     "socks5",
						Username: "proxyuser",
						Password: "proxypass",
					},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "socks5-proxy"`,
		`type = "tcp"`,
		`remotePort = 1080`,
		`plugin = "socks5"`,
		`plugin.username = "proxyuser"`,
		`plugin.password = "proxypass"`,
	},
	wantNotContain: []string{
		`localIP`,
		`localPort`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add plugin to TCP template**

Update TCP section:

```go
{{ if eq $upstream.Type 1 }}
name = "{{ $upstream.Name }}"
type = "tcp"
{{ if $upstream.TCP.Host }}
localIP = "{{ $upstream.TCP.Host }}"
{{ end }}
{{ if $upstream.TCP.Port }}
localPort = {{ $upstream.TCP.Port }}
{{ end }}
remotePort = {{ $upstream.TCP.ServerPort }}

{{ if $upstream.TCP.Plugin }}
plugin = "{{ $upstream.TCP.Plugin.Type }}"

{{ if eq $upstream.TCP.Plugin.Type "socks5" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.username = "{{ $upstream.TCP.Plugin.Username }}"
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.password = "{{ $upstream.TCP.Plugin.Password }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "http_proxy" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.httpUser = "{{ $upstream.TCP.Plugin.Username }}"
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.httpPassword = "{{ $upstream.TCP.Plugin.Password }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "static_file" }}
plugin.localPath = "{{ $upstream.TCP.Plugin.LocalPath }}"
{{ if $upstream.TCP.Plugin.StripPrefix }}
plugin.stripPrefix = "{{ $upstream.TCP.Plugin.StripPrefix }}"
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPUser }}
plugin.httpUser = "{{ $upstream.TCP.Plugin.HTTPUser }}"
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPPassword }}
plugin.httpPassword = "{{ $upstream.TCP.Plugin.HTTPPassword }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "unix_domain_socket" }}
plugin.unixPath = "{{ $upstream.TCP.Plugin.UnixPath }}"
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "https2http" }}
plugin.localAddr = "{{ $upstream.TCP.Plugin.LocalAddr }}"
{{ end }}
{{ end }}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 6: Add more plugin tests**

```go
{
	name: "TCP upstream - static_file plugin",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "file-server",
				Type: 1,
				TCP: models.Upstream_TCP{
					ServerPort: 8080,
					Plugin: &models.PluginConfig{
						Type:        "static_file",
						LocalPath:   "/data/public",
						StripPrefix: "/download",
						HTTPUser:    "admin",
						HTTPPassword: "secret",
					},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`plugin = "static_file"`,
		`plugin.localPath = "/data/public"`,
		`plugin.stripPrefix = "/download"`,
		`plugin.httpUser = "admin"`,
	},
},
{
	name: "TCP upstream - unix_domain_socket plugin",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "docker-api",
				Type: 1,
				TCP: models.Upstream_TCP{
					ServerPort: 2375,
					Plugin: &models.PluginConfig{
						Type:     "unix_domain_socket",
						UnixPath: "/var/run/docker.sock",
					},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`plugin = "unix_domain_socket"`,
		`plugin.unixPath = "/var/run/docker.sock"`,
	},
},
```

**Step 7: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add plugin support for TCP upstreams"
```

---

## Task 5: Add TCPMUX Upstream Type

**Files:**
- Modify: `api/v1alpha1/upstream_types.go`

**Step 1: Add TCPMUX type**

```go
type UpstreamSpec_TCPMUX struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:Enum=httpconnect
	Multiplexer   string   `json:"multiplexer"`
	CustomDomains []string `json:"customDomains"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}
```

**Step 2: Add TCPMUX to UpstreamSpec**

```go
type UpstreamSpec struct {
	Client string `json:"client"`
	// +optional
	TCP *UpstreamSpec_TCP `json:"tcp"`
	// +optional
	UDP *UpstreamSpec_UDP `json:"udp"`
	// +optional
	STCP *UpstreamSpec_STCP `json:"stcp"`
	// +optional
	XTCP *UpstreamSpec_XTCP `json:"xtcp"`
	// +optional
	HTTP *UpstreamSpec_HTTP `json:"http"`
	// +optional
	HTTPS *UpstreamSpec_HTTPS `json:"https"`
	// +optional
	TCPMUX *UpstreamSpec_TCPMUX `json:"tcpmux"`
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/upstream_types.go config/crd/
git commit -m "feat(api): add TCPMUX upstream type"
```

---

## Task 6: Add TCPMUX to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`

**Step 1: Add TCPMUX constant and model**

```go
const (
	TCP    UpstreamType = iota
	UDP    UpstreamType = iota
	STCP   UpstreamType = iota
	XTCP   UpstreamType = iota
	HTTP   UpstreamType = iota
	HTTPS  UpstreamType = iota
	TCPMUX UpstreamType = iota
)

type Upstream_TCPMUX struct {
	Host          string
	Port          int
	Multiplexer   string
	CustomDomains []string
	Transport     *Upstream_TCP_Transport
}

type Upstream struct {
	Name   string
	Type   UpstreamType
	TCP    Upstream_TCP
	UDP    Upstream_UDP
	STCP   Upstream_STCP
	XTCP   Upstream_STCP
	HTTP   Upstream_HTTP
	HTTPS  Upstream_HTTPS
	TCPMUX Upstream_TCPMUX
}
```

**Step 2: Write failing test**

```go
{
	name: "TCPMUX upstream",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "mux-service",
				Type: 7, // TCPMUX
				TCPMUX: models.Upstream_TCPMUX{
					Host:          "internal-service.default.svc",
					Port:          8080,
					Multiplexer:   "httpconnect",
					CustomDomains: []string{"mux.example.com"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`[[proxies]]`,
		`name = "mux-service"`,
		`type = "tcpmux"`,
		`multiplexer = "httpconnect"`,
		`localIP = "internal-service.default.svc"`,
		`localPort = 8080`,
		`customDomains = ["mux.example.com"]`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add TCPMUX template**

```go
{{ if eq $upstream.Type 7 }}
name = "{{ $upstream.Name }}"
type = "tcpmux"
multiplexer = "{{ $upstream.TCPMUX.Multiplexer }}"
localIP = "{{ $upstream.TCPMUX.Host }}"
localPort = {{ $upstream.TCPMUX.Port }}

{{ if $upstream.TCPMUX.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.TCPMUX.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]
{{ end }}

{{ if $upstream.TCPMUX.Transport }}
transport.useEncryption = {{ $upstream.TCPMUX.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.TCPMUX.Transport.UseCompression }}
{{ end }}
{{ end }}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 6: Update NewConfig for TCPMUX**

```go
if upstreamObject.Spec.TCPMUX != nil {
	upstream.Type = 7
	upstream.TCPMUX.Host = upstreamObject.Spec.TCPMUX.Host
	upstream.TCPMUX.Port = upstreamObject.Spec.TCPMUX.Port
	upstream.TCPMUX.Multiplexer = upstreamObject.Spec.TCPMUX.Multiplexer
	upstream.TCPMUX.CustomDomains = upstreamObject.Spec.TCPMUX.CustomDomains

	if upstreamObject.Spec.TCPMUX.Transport != nil {
		upstream.TCPMUX.Transport = &Upstream_TCP_Transport{
			UseCompression: upstreamObject.Spec.TCPMUX.Transport.UseCompression,
			UseEncryption:  upstreamObject.Spec.TCPMUX.Transport.UseEncryption,
		}
	}
}
```

**Step 7: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add TCPMUX upstream support"
```

---

## Task 7: Add Client Transport Configuration

**Files:**
- Modify: `api/v1alpha1/client_types.go`

**Step 1: Add Transport type to Client**

```go
type ClientSpec_Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	Protocol       *string                          `json:"protocol"`
	Authentication ClientSpec_Server_Authentication `json:"authentication"`
	AdminServer    *ClientSpec_Server_AdminServer   `json:"adminServer,omitempty"`
	// +optional
	STUNServer *string `json:"stunServer"`
	// +optional
	TLS *ClientSpec_Server_TLS `json:"tls,omitempty"`
	// +optional
	Transport *ClientSpec_Server_Transport `json:"transport,omitempty"`
}

type ClientSpec_Server_Transport struct {
	// +optional
	// +kubebuilder:default=1
	PoolCount int `json:"poolCount,omitempty"`
	// +optional
	// +kubebuilder:default=true
	TCPMux *bool `json:"tcpMux,omitempty"`
	// +optional
	// +kubebuilder:default="10s"
	DialServerTimeout string `json:"dialServerTimeout,omitempty"`
	// +optional
	// +kubebuilder:default="-1s"
	DialServerKeepalive string `json:"dialServerKeepalive,omitempty"`
	// +optional
	ConnectServerLocalIP string `json:"connectServerLocalIP,omitempty"`
}
```

**Step 2: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 3: Commit**

```bash
git add api/v1alpha1/client_types.go config/crd/
git commit -m "feat(api): add Transport configuration to Client"
```

---

## Task 8: Add Client Transport to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`

**Step 1: Add Transport to Common model**

```go
type Common struct {
	ServerAddress        string
	ServerPort           int
	ServerProtocol       string
	ServerAuthentication ServerAuthentication
	AdminAddress         string
	AdminPort            int
	AdminUsername        string
	AdminPassword        string
	STUNServer           *string
	TLS                  *TLSConfig
	PprofEnable          bool
	Transport            *TransportConfig
}

type TransportConfig struct {
	PoolCount            int
	TCPMux               bool
	DialServerTimeout    string
	DialServerKeepalive  string
	ConnectServerLocalIP string
}
```

**Step 2: Write failing test**

```go
{
	name: "common config with transport tuning",
	config: models.Config{
		Common: models.Common{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			Transport: &models.TransportConfig{
				PoolCount:            5,
				TCPMux:               true,
				DialServerTimeout:    "15s",
				DialServerKeepalive:  "30s",
				ConnectServerLocalIP: "10.0.0.5",
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`transport.poolCount = 5`,
		`transport.tcpMux = true`,
		`transport.dialServerTimeout = "15s"`,
		`transport.dialServerKeepalive = "30s"`,
		`transport.connectServerLocalIP = "10.0.0.5"`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add transport to template**

Add after TLS section:

```go
{{ if .Common.Transport }}
transport.poolCount = {{ .Common.Transport.PoolCount }}
transport.tcpMux = {{ .Common.Transport.TCPMux }}
{{ if .Common.Transport.DialServerTimeout }}
transport.dialServerTimeout = "{{ .Common.Transport.DialServerTimeout }}"
{{ end }}
{{ if .Common.Transport.DialServerKeepalive }}
transport.dialServerKeepalive = "{{ .Common.Transport.DialServerKeepalive }}"
{{ end }}
{{ if .Common.Transport.ConnectServerLocalIP }}
transport.connectServerLocalIP = "{{ .Common.Transport.ConnectServerLocalIP }}"
{{ end }}
{{ end }}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 6: Update NewConfig for Transport**

```go
if clientObject.Spec.Server.Transport != nil {
	config.Common.Transport = &TransportConfig{
		PoolCount:            clientObject.Spec.Server.Transport.PoolCount,
		DialServerTimeout:    clientObject.Spec.Server.Transport.DialServerTimeout,
		DialServerKeepalive:  clientObject.Spec.Server.Transport.DialServerKeepalive,
		ConnectServerLocalIP: clientObject.Spec.Server.Transport.ConnectServerLocalIP,
	}

	if clientObject.Spec.Server.Transport.TCPMux != nil {
		config.Common.Transport.TCPMux = *clientObject.Spec.Server.Transport.TCPMux
	} else {
		config.Common.Transport.TCPMux = true // default
	}
}
```

**Step 7: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add client transport configuration"
```

---

## Task 9: Update NewConfig for Plugin

**Files:**
- Modify: `pkg/client/models/config.go`

**Step 1: Add plugin handling to TCP NewConfig**

```go
if upstreamObject.Spec.TCP.Plugin != nil {
	upstream.TCP.Plugin = &PluginConfig{
		Type:      upstreamObject.Spec.TCP.Plugin.Type,
		LocalPath: upstreamObject.Spec.TCP.Plugin.LocalPath,
		StripPrefix: upstreamObject.Spec.TCP.Plugin.StripPrefix,
		LocalAddr: upstreamObject.Spec.TCP.Plugin.LocalAddr,
		UnixPath:  upstreamObject.Spec.TCP.Plugin.UnixPath,
	}

	// Fetch username from secret
	if upstreamObject.Spec.TCP.Plugin.Username != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      upstreamObject.Spec.TCP.Plugin.Username.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.Username.Secret.Key]; ok {
				upstream.TCP.Plugin.Username = string(val)
			}
		}
	}

	// Fetch password from secret
	if upstreamObject.Spec.TCP.Plugin.Password != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      upstreamObject.Spec.TCP.Plugin.Password.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.Password.Secret.Key]; ok {
				upstream.TCP.Plugin.Password = string(val)
			}
		}
	}

	// Similar for HTTPUser and HTTPPassword
	if upstreamObject.Spec.TCP.Plugin.HTTPUser != nil {
		// ... fetch from secret ...
	}
	if upstreamObject.Spec.TCP.Plugin.HTTPPassword != nil {
		// ... fetch from secret ...
	}
}
```

**Step 2: Run tests**

Run: `make test`
Expected: All PASS

**Step 3: Commit**

```bash
git add pkg/client/models/config.go
git commit -m "feat: handle plugin configuration in NewConfig"
```

---

## Task 10: Add Examples and Final Test

**Files:**
- Create: `examples/advanced/`

**Step 1: Create advanced examples directory**

Run: `mkdir -p examples/advanced`

**Step 2: Create load balancing example**

Create `examples/advanced/load-balancing.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-node-1
spec:
  client: advanced-client
  tcp:
    host: api-1.default.svc
    port: 8080
    server:
      port: 9000
    loadBalancer:
      group: "api-cluster"
      groupKey:
        secret:
          name: lb-secret
          key: groupKey
---
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-node-2
spec:
  client: advanced-client
  tcp:
    host: api-2.default.svc
    port: 8080
    server:
      port: 9000
    loadBalancer:
      group: "api-cluster"
      groupKey:
        secret:
          name: lb-secret
          key: groupKey
```

**Step 3: Create plugin examples**

Create `examples/advanced/plugins.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: socks5-proxy
spec:
  client: advanced-client
  tcp:
    server:
      port: 1080
    plugin:
      type: socks5
      username:
        secret:
          name: socks-auth
          key: username
      password:
        secret:
          name: socks-auth
          key: password
---
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: docker-api
spec:
  client: advanced-client
  tcp:
    server:
      port: 2375
    plugin:
      type: unix_domain_socket
      unixPath: "/var/run/docker.sock"
```

**Step 4: Create TCPMUX example**

Create `examples/advanced/tcpmux.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: mux-service
spec:
  client: advanced-client
  tcpmux:
    host: internal-service.default.svc
    port: 8080
    multiplexer: httpconnect
    customDomains:
      - "mux.example.com"
```

**Step 5: Create client with transport tuning**

Create `examples/advanced/client-transport.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: optimized-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    transport:
      poolCount: 5
      tcpMux: true
      dialServerTimeout: "15s"
      dialServerKeepalive: "30s"
```

**Step 6: Update Helm CRDs**

Run: `cp config/crd/bases/*.yaml charts/frp-operator/crds/`

**Step 7: Run all tests**

Run: `make test`
Expected: All PASS

**Step 8: Build and verify**

Run: `make build`
Expected: No errors

**Step 9: Commit**

```bash
git add examples/advanced/ charts/frp-operator/crds/
git commit -m "feat: complete Phase 4 - advanced features"
```

---

## Summary

Phase 4 adds advanced features:
- **Load Balancing**: Distribute traffic across multiple backends using group/groupKey
- **Plugins**: socks5, http_proxy, static_file, unix_domain_socket, https2http
- **TCPMUX**: Multiplex services over HTTP CONNECT
- **Client Transport**: Connection pooling, TCP mux, timeouts, keepalive

Files modified:
- `api/v1alpha1/upstream_types.go` - LoadBalancer, Plugin, TCPMUX
- `api/v1alpha1/client_types.go` - Transport configuration
- `pkg/client/models/config.go` - Model structs, NewConfig
- `pkg/client/utils/template.go` - TOML templates
- `examples/advanced/` - Example manifests
