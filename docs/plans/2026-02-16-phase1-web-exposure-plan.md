# Phase 1: Web Service Exposure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add HTTP and HTTPS upstream proxy types to expose web services with custom domains, URL routing, and header manipulation.

**Architecture:** Extend UpstreamSpec with HTTP/HTTPS types, add corresponding model structs, update the TOML template, and update NewConfig to transform CRD specs to internal models.

**Tech Stack:** Go, Kubebuilder v3, controller-runtime, text/template

---

## Task 1: Add HTTP/HTTPS Types to API

**Files:**
- Modify: `api/v1alpha1/upstream_types.go:24-35`
- Modify: `api/v1alpha1/types.go`

**Step 1: Add SecretRef type to types.go**

Add to `api/v1alpha1/types.go`:

```go
package v1alpha1

type Secret struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type SecretRef struct {
	Secret Secret `json:"secret"`
}
```

**Step 2: Run `make generate` to verify no errors**

Run: `make generate`
Expected: No errors

**Step 3: Add HTTP upstream type to upstream_types.go**

Add after the XTCP type definition in `api/v1alpha1/upstream_types.go`:

```go
type UpstreamSpec_HTTP struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +optional
	Subdomain string `json:"subdomain,omitempty"`
	// +optional
	CustomDomains []string `json:"customDomains,omitempty"`
	// +optional
	Locations []string `json:"locations,omitempty"`
	// +optional
	HostHeaderRewrite string `json:"hostHeaderRewrite,omitempty"`
	// +optional
	RequestHeaders *HTTPHeaders `json:"requestHeaders,omitempty"`
	// +optional
	ResponseHeaders *HTTPHeaders `json:"responseHeaders,omitempty"`
	// +optional
	HTTPUser *SecretRef `json:"httpUser,omitempty"`
	// +optional
	HTTPPassword *SecretRef `json:"httpPassword,omitempty"`
	// +optional
	HealthCheck *UpstreamSpec_HTTP_HealthCheck `json:"healthCheck,omitempty"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}

type HTTPHeaders struct {
	Set map[string]string `json:"set,omitempty"`
}

type UpstreamSpec_HTTP_HealthCheck struct {
	// +kubebuilder:validation:Enum=http
	Type            string `json:"type"`
	Path            string `json:"path"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	IntervalSeconds int    `json:"intervalSeconds"`
	MaxFailed       int    `json:"maxFailed"`
}

type UpstreamSpec_HTTPS struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	CustomDomains []string `json:"customDomains"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol,omitempty"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}
```

**Step 4: Add HTTP/HTTPS fields to UpstreamSpec**

Modify `UpstreamSpec` in `api/v1alpha1/upstream_types.go`:

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
}
```

**Step 5: Run code generation**

Run: `make generate && make manifests`
Expected: No errors, CRD files updated

**Step 6: Commit**

```bash
git add api/v1alpha1/upstream_types.go api/v1alpha1/types.go config/crd/
git commit -m "feat(api): add HTTP and HTTPS upstream types"
```

---

## Task 2: Add HTTP/HTTPS Model Structs

**Files:**
- Modify: `pkg/client/models/config.go:98-115`

**Step 1: Add HTTP/HTTPS UpstreamType constants**

Modify the UpstreamType constants in `pkg/client/models/config.go`:

```go
const (
	TCP  UpstreamType = iota
	UDP  UpstreamType = iota
	STCP UpstreamType = iota
	XTCP UpstreamType = iota
	HTTP UpstreamType = iota
	HTTPS UpstreamType = iota
)
```

**Step 2: Add HTTP/HTTPS model structs**

Add after `Upstream_UDP` struct:

```go
type Upstream_HTTP struct {
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
	HealthCheck       *Upstream_HTTP_HealthCheck
	Transport         *Upstream_TCP_Transport
}

type Upstream_HTTP_HealthCheck struct {
	Type            string
	Path            string
	TimeoutSeconds  int
	IntervalSeconds int
	MaxFailed       int
}

type Upstream_HTTPS struct {
	Host          string
	Port          int
	CustomDomains []string
	ProxyProtocol *string
	Transport     *Upstream_TCP_Transport
}
```

**Step 3: Add HTTP/HTTPS fields to Upstream struct**

Modify the `Upstream` struct:

```go
type Upstream struct {
	Name  string
	Type  UpstreamType
	TCP   Upstream_TCP
	UDP   Upstream_UDP
	STCP  Upstream_STCP
	XTCP  Upstream_STCP
	HTTP  Upstream_HTTP
	HTTPS Upstream_HTTPS
}
```

**Step 4: Run tests to verify no compilation errors**

Run: `go build ./...`
Expected: No errors

**Step 5: Commit**

```bash
git add pkg/client/models/config.go
git commit -m "feat(models): add HTTP and HTTPS upstream model structs"
```

---

## Task 3: Add HTTP TOML Template

**Files:**
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Write failing test for HTTP upstream**

Add to `pkg/client/builder/configuration_builder_test.go`:

```go
{
	name: "HTTP upstream - basic with subdomain",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "my-http-service",
				Type: 5, // HTTP
				HTTP: models.Upstream_HTTP{
					Host:      "web-service.default.svc",
					Port:      8080,
					Subdomain: "webapp",
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`[[proxies]]`,
		`name = "my-http-service"`,
		`type = "http"`,
		`localIP = "web-service.default.svc"`,
		`localPort = 8080`,
		`subdomain = "webapp"`,
	},
	wantNotContain: []string{
		`customDomains`,
		`locations`,
		`hostHeaderRewrite`,
	},
},
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL (template doesn't handle Type 5)

**Step 3: Add HTTP template to template.go**

Add after the XTCP template block (after `{{ end }}` for Type 4):

```go
{{ if eq $upstream.Type 5 }}
name = "{{ $upstream.Name }}"
type = "http"
localIP = "{{ $upstream.HTTP.Host }}"
localPort = {{ $upstream.HTTP.Port }}

{{ if $upstream.HTTP.Subdomain }}
subdomain = "{{ $upstream.HTTP.Subdomain }}"
{{ end }}

{{ if $upstream.HTTP.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTP.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTP.Locations }}
locations = [{{ range $i, $l := $upstream.HTTP.Locations }}{{ if $i }}, {{ end }}"{{ $l }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTP.HostHeaderRewrite }}
hostHeaderRewrite = "{{ $upstream.HTTP.HostHeaderRewrite }}"
{{ end }}

{{ if $upstream.HTTP.RequestHeaders }}
{{ range $k, $v := $upstream.HTTP.RequestHeaders }}
requestHeaders.set.{{ $k }} = "{{ $v }}"
{{ end }}
{{ end }}

{{ if $upstream.HTTP.ResponseHeaders }}
{{ range $k, $v := $upstream.HTTP.ResponseHeaders }}
responseHeaders.set.{{ $k }} = "{{ $v }}"
{{ end }}
{{ end }}

{{ if $upstream.HTTP.HTTPUser }}
httpUser = "{{ $upstream.HTTP.HTTPUser }}"
{{ end }}
{{ if $upstream.HTTP.HTTPPassword }}
httpPassword = "{{ $upstream.HTTP.HTTPPassword }}"
{{ end }}

{{ if $upstream.HTTP.HealthCheck }}
healthCheck.type = "{{ $upstream.HTTP.HealthCheck.Type }}"
healthCheck.path = "{{ $upstream.HTTP.HealthCheck.Path }}"
healthCheck.timeoutSeconds = {{ $upstream.HTTP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.HTTP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.HTTP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.HTTP.Transport }}
transport.useEncryption = {{ $upstream.HTTP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.HTTP.Transport.UseCompression }}
{{ if $upstream.HTTP.Transport.BandwdithLimit }}
{{ if $upstream.HTTP.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.HTTP.Transport.BandwdithLimit.Limit }}{{ $upstream.HTTP.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.HTTP.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.HTTP.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat(template): add HTTP upstream TOML template"
```

---

## Task 4: Add More HTTP Template Tests

**Files:**
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Add test for HTTP with custom domains**

```go
{
	name: "HTTP upstream - with custom domains",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "http-custom-domains",
				Type: 5,
				HTTP: models.Upstream_HTTP{
					Host:          "api-service.default.svc",
					Port:          8080,
					CustomDomains: []string{"api.example.com", "api2.example.com"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "http-custom-domains"`,
		`type = "http"`,
		`customDomains = ["api.example.com", "api2.example.com"]`,
	},
},
```

**Step 2: Add test for HTTP with locations and headers**

```go
{
	name: "HTTP upstream - with locations and headers",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "http-full",
				Type: 5,
				HTTP: models.Upstream_HTTP{
					Host:              "api-service.default.svc",
					Port:              8080,
					Subdomain:         "api",
					Locations:         []string{"/v1", "/v2"},
					HostHeaderRewrite: "internal-api.local",
					RequestHeaders:    map[string]string{"X-Forwarded-By": "frp-operator"},
					ResponseHeaders:   map[string]string{"X-Frame-Options": "DENY"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "http-full"`,
		`subdomain = "api"`,
		`locations = ["/v1", "/v2"]`,
		`hostHeaderRewrite = "internal-api.local"`,
		`requestHeaders.set.X-Forwarded-By = "frp-operator"`,
		`responseHeaders.set.X-Frame-Options = "DENY"`,
	},
},
```

**Step 3: Add test for HTTP with basic auth and health check**

```go
{
	name: "HTTP upstream - with auth and health check",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "http-auth",
				Type: 5,
				HTTP: models.Upstream_HTTP{
					Host:         "protected.default.svc",
					Port:         8080,
					Subdomain:    "admin",
					HTTPUser:     "admin",
					HTTPPassword: "secret123",
					HealthCheck: &models.Upstream_HTTP_HealthCheck{
						Type:            "http",
						Path:            "/health",
						TimeoutSeconds:  5,
						IntervalSeconds: 10,
						MaxFailed:       3,
					},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "http-auth"`,
		`httpUser = "admin"`,
		`httpPassword = "secret123"`,
		`healthCheck.type = "http"`,
		`healthCheck.path = "/health"`,
		`healthCheck.timeoutSeconds = 5`,
	},
},
```

**Step 4: Run all tests**

Run: `go test ./pkg/client/builder/... -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/client/builder/configuration_builder_test.go
git commit -m "test(builder): add comprehensive HTTP upstream tests"
```

---

## Task 5: Add HTTPS TOML Template

**Files:**
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Write failing test for HTTPS upstream**

Add to test file:

```go
{
	name: "HTTPS upstream - basic",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "my-https-service",
				Type: 6, // HTTPS
				HTTPS: models.Upstream_HTTPS{
					Host:          "secure-app.default.svc",
					Port:          443,
					CustomDomains: []string{"secure.example.com"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`[[proxies]]`,
		`name = "my-https-service"`,
		`type = "https"`,
		`localIP = "secure-app.default.svc"`,
		`localPort = 443`,
		`customDomains = ["secure.example.com"]`,
	},
},
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 3: Add HTTPS template**

Add after the HTTP template block:

```go
{{ if eq $upstream.Type 6 }}
name = "{{ $upstream.Name }}"
type = "https"
localIP = "{{ $upstream.HTTPS.Host }}"
localPort = {{ $upstream.HTTPS.Port }}

{{ if $upstream.HTTPS.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTPS.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTPS.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.HTTPS.ProxyProtocol }}"
{{ end }}

{{ if $upstream.HTTPS.Transport }}
transport.useEncryption = {{ $upstream.HTTPS.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.HTTPS.Transport.UseCompression }}
{{ if $upstream.HTTPS.Transport.BandwdithLimit }}
{{ if $upstream.HTTPS.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.HTTPS.Transport.BandwdithLimit.Limit }}{{ $upstream.HTTPS.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ end }}
{{ end }}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 5: Add test for HTTPS with proxy protocol**

```go
{
	name: "HTTPS upstream - with proxy protocol",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "https-proxy-protocol",
				Type: 6,
				HTTPS: models.Upstream_HTTPS{
					Host:          "app.default.svc",
					Port:          443,
					CustomDomains: []string{"app.example.com"},
					ProxyProtocol: stringPtr("v2"),
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "https-proxy-protocol"`,
		`type = "https"`,
		`transport.proxyProtocolVersion = "v2"`,
	},
},
```

**Step 6: Run all tests**

Run: `go test ./pkg/client/builder/... -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat(template): add HTTPS upstream TOML template"
```

---

## Task 6: Update NewConfig for HTTP/HTTPS

**Files:**
- Modify: `pkg/client/models/config.go:319-480`
- Test: `pkg/client/models/config_test.go`

**Step 1: Add HTTP handling to NewConfig**

Add after the XTCP handling block in `NewConfig`:

```go
if upstreamObject.Spec.HTTP != nil {
	upstream.Type = 5
	upstream.HTTP.Host = upstreamObject.Spec.HTTP.Host
	upstream.HTTP.Port = upstreamObject.Spec.HTTP.Port

	if upstreamObject.Spec.HTTP.Subdomain != "" {
		upstream.HTTP.Subdomain = upstreamObject.Spec.HTTP.Subdomain
	}

	if len(upstreamObject.Spec.HTTP.CustomDomains) > 0 {
		upstream.HTTP.CustomDomains = upstreamObject.Spec.HTTP.CustomDomains
	}

	if len(upstreamObject.Spec.HTTP.Locations) > 0 {
		upstream.HTTP.Locations = upstreamObject.Spec.HTTP.Locations
	}

	if upstreamObject.Spec.HTTP.HostHeaderRewrite != "" {
		upstream.HTTP.HostHeaderRewrite = upstreamObject.Spec.HTTP.HostHeaderRewrite
	}

	if upstreamObject.Spec.HTTP.RequestHeaders != nil {
		upstream.HTTP.RequestHeaders = upstreamObject.Spec.HTTP.RequestHeaders.Set
	}

	if upstreamObject.Spec.HTTP.ResponseHeaders != nil {
		upstream.HTTP.ResponseHeaders = upstreamObject.Spec.HTTP.ResponseHeaders.Set
	}

	// Fetch HTTP user from secret
	if upstreamObject.Spec.HTTP.HTTPUser != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      upstreamObject.Spec.HTTP.HTTPUser.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[upstreamObject.Spec.HTTP.HTTPUser.Secret.Key]; ok {
				upstream.HTTP.HTTPUser = string(val)
			}
		}
	}

	// Fetch HTTP password from secret
	if upstreamObject.Spec.HTTP.HTTPPassword != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      upstreamObject.Spec.HTTP.HTTPPassword.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[upstreamObject.Spec.HTTP.HTTPPassword.Secret.Key]; ok {
				upstream.HTTP.HTTPPassword = string(val)
			}
		}
	}

	if upstreamObject.Spec.HTTP.HealthCheck != nil {
		upstream.HTTP.HealthCheck = &Upstream_HTTP_HealthCheck{
			Type:            upstreamObject.Spec.HTTP.HealthCheck.Type,
			Path:            upstreamObject.Spec.HTTP.HealthCheck.Path,
			TimeoutSeconds:  upstreamObject.Spec.HTTP.HealthCheck.TimeoutSeconds,
			IntervalSeconds: upstreamObject.Spec.HTTP.HealthCheck.IntervalSeconds,
			MaxFailed:       upstreamObject.Spec.HTTP.HealthCheck.MaxFailed,
		}
	}

	if upstreamObject.Spec.HTTP.Transport != nil {
		upstream.HTTP.Transport = &Upstream_TCP_Transport{
			UseCompression: upstreamObject.Spec.HTTP.Transport.UseCompression,
			UseEncryption:  upstreamObject.Spec.HTTP.Transport.UseEncryption,
		}

		if upstreamObject.Spec.HTTP.Transport.ProxyURL != nil {
			upstream.HTTP.Transport.ProxyURL = upstreamObject.Spec.HTTP.Transport.ProxyURL
		}

		if upstreamObject.Spec.HTTP.Transport.BandwdithLimit != nil {
			upstream.HTTP.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
				Enabled: upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Enabled,
				Limit:   upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Limit,
				Type:    upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Type,
			}
		}
	}
}
```

**Step 2: Add HTTPS handling to NewConfig**

```go
if upstreamObject.Spec.HTTPS != nil {
	upstream.Type = 6
	upstream.HTTPS.Host = upstreamObject.Spec.HTTPS.Host
	upstream.HTTPS.Port = upstreamObject.Spec.HTTPS.Port
	upstream.HTTPS.CustomDomains = upstreamObject.Spec.HTTPS.CustomDomains

	if upstreamObject.Spec.HTTPS.ProxyProtocol != nil {
		upstream.HTTPS.ProxyProtocol = upstreamObject.Spec.HTTPS.ProxyProtocol
	}

	if upstreamObject.Spec.HTTPS.Transport != nil {
		upstream.HTTPS.Transport = &Upstream_TCP_Transport{
			UseCompression: upstreamObject.Spec.HTTPS.Transport.UseCompression,
			UseEncryption:  upstreamObject.Spec.HTTPS.Transport.UseEncryption,
		}

		if upstreamObject.Spec.HTTPS.Transport.BandwdithLimit != nil {
			upstream.HTTPS.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
				Enabled: upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Enabled,
				Limit:   upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Limit,
				Type:    upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Type,
			}
		}
	}
}
```

**Step 3: Update validation error message**

Update the validation at line 325-327:

```go
if upstreamObject.Spec.TCP == nil && upstreamObject.Spec.UDP == nil &&
   upstreamObject.Spec.STCP == nil && upstreamObject.Spec.XTCP == nil &&
   upstreamObject.Spec.HTTP == nil && upstreamObject.Spec.HTTPS == nil {
	return config, errors.NewBadRequest("TCP, UDP, STCP, XTCP, HTTP, or HTTPS upstream is required")
}
```

**Step 4: Run all tests**

Run: `make test`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/client/models/config.go
git commit -m "feat(models): add HTTP/HTTPS handling in NewConfig"
```

---

## Task 7: Add Example Manifests

**Files:**
- Create: `examples/http/client.yaml`
- Create: `examples/http/secret.yaml`
- Create: `examples/http/upstream-http.yaml`
- Create: `examples/http/upstream-https.yaml`

**Step 1: Create examples/http directory**

Run: `mkdir -p examples/http`

**Step 2: Create client.yaml**

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: http-example-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
```

**Step 3: Create secret.yaml**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: frp-token
type: Opaque
stringData:
  token: "your-frp-server-token"
---
apiVersion: v1
kind: Secret
metadata:
  name: http-auth
type: Opaque
stringData:
  username: "admin"
  password: "secret123"
```

**Step 4: Create upstream-http.yaml**

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: webapp-http
spec:
  client: http-example-client
  http:
    host: my-service.default.svc
    port: 8080
    subdomain: "webapp"
    transport:
      useEncryption: true
---
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-http
spec:
  client: http-example-client
  http:
    host: api-service.default.svc
    port: 8080
    customDomains:
      - "api.example.com"
    locations:
      - "/v1"
      - "/v2"
    hostHeaderRewrite: "internal-api.local"
    requestHeaders:
      set:
        X-Forwarded-By: "frp-operator"
    healthCheck:
      type: http
      path: /health
      timeoutSeconds: 5
      intervalSeconds: 10
      maxFailed: 3
```

**Step 5: Create upstream-https.yaml**

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: secure-app-https
spec:
  client: http-example-client
  https:
    host: secure-app.default.svc
    port: 443
    customDomains:
      - "secure.example.com"
    proxyProtocol: "v2"
```

**Step 6: Commit**

```bash
git add examples/http/
git commit -m "docs(examples): add HTTP/HTTPS upstream examples"
```

---

## Task 8: Update Helm Chart CRDs

**Files:**
- Modify: `charts/frp-operator/crds/`

**Step 1: Run make manifests to update CRDs**

Run: `make manifests`

**Step 2: Copy CRDs to Helm chart**

Run: `cp config/crd/bases/*.yaml charts/frp-operator/crds/`

**Step 3: Run all tests**

Run: `make test`
Expected: All PASS

**Step 4: Commit**

```bash
git add charts/frp-operator/crds/
git commit -m "chore(helm): update CRDs with HTTP/HTTPS upstream types"
```

---

## Task 9: Final Integration Test

**Step 1: Build the operator**

Run: `make build`
Expected: No errors

**Step 2: Install CRDs locally**

Run: `make install`
Expected: CRDs installed

**Step 3: Run operator locally**

Run: `make run`
Expected: Operator starts without errors

**Step 4: Apply example manifests (in another terminal)**

Run: `kubectl apply -f examples/http/`
Expected: Resources created

**Step 5: Verify ConfigMap contains HTTP config**

Run: `kubectl get configmap http-example-client-frpc-config -o yaml`
Expected: Contains `type = "http"` and `subdomain = "webapp"`

**Step 6: Clean up**

Run: `kubectl delete -f examples/http/`

**Step 7: Final commit**

```bash
git add -A
git commit -m "feat: complete Phase 1 - HTTP/HTTPS upstream support"
```

---

## Summary

Phase 1 adds HTTP and HTTPS upstream proxy types with:
- Custom domains and subdomain routing
- URL path-based routing (locations)
- Header manipulation (request/response)
- Basic HTTP authentication
- HTTP health checks
- Full transport options (encryption, compression, bandwidth)

Files modified:
- `api/v1alpha1/upstream_types.go` - New HTTP/HTTPS types
- `api/v1alpha1/types.go` - SecretRef type
- `pkg/client/models/config.go` - Model structs and NewConfig
- `pkg/client/utils/template.go` - TOML templates
- `pkg/client/builder/configuration_builder_test.go` - Tests
- `examples/http/` - Example manifests
- `charts/frp-operator/crds/` - Updated CRDs
