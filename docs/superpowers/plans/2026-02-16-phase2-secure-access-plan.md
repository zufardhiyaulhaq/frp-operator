# Phase 2: Secure Access Control Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add allowUsers for STCP/XTCP access control, OIDC authentication, and TLS client certificate configuration.

**Architecture:** Extend STCP/XTCP types with allowUsers field, add OIDC authentication option to Client, add TLS configuration block, update templates and models accordingly.

**Tech Stack:** Go, Kubebuilder v3, controller-runtime, text/template

---

## Task 1: Add allowUsers to STCP/XTCP Types

**Files:**
- Modify: `api/v1alpha1/upstream_types.go:36-68`

**Step 1: Add allowUsers field to UpstreamSpec_STCP**

Modify `UpstreamSpec_STCP` in `api/v1alpha1/upstream_types.go`:

```go
type UpstreamSpec_STCP struct {
	Host      string                      `json:"host"`
	Port      int                         `json:"port"`
	SecretKey UpstreamSpec_STCP_SecretKey `json:"secretKey"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport"`
	// +optional
	// AllowUsers specifies which FRP users can connect to this tunnel.
	// Use "*" to allow any user. Empty means only the same user.
	AllowUsers []string `json:"allowUsers,omitempty"`
}
```

**Step 2: Add allowUsers field to UpstreamSpec_XTCP**

Modify `UpstreamSpec_XTCP`:

```go
type UpstreamSpec_XTCP struct {
	Host      string                      `json:"host"`
	Port      int                         `json:"port"`
	SecretKey UpstreamSpec_XTCP_SecretKey `json:"secretKey"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport"`
	// +optional
	AllowUsers []string `json:"allowUsers,omitempty"`
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/upstream_types.go config/crd/
git commit -m "feat(api): add allowUsers field to STCP/XTCP upstream types"
```

---

## Task 2: Add allowUsers to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Add AllowUsers to Upstream_STCP model**

Modify `Upstream_STCP` in `pkg/client/models/config.go`:

```go
type Upstream_STCP struct {
	Host          string
	Port          int
	SecretKey     string
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	AllowUsers    []string
}
```

**Step 2: Write failing test for STCP with allowUsers**

Add to `pkg/client/builder/configuration_builder_test.go`:

```go
{
	name: "STCP upstream - with allowUsers",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "stcp-with-allowusers",
				Type: 3,
				STCP: models.Upstream_STCP{
					Host:       "127.0.0.1",
					Port:       22,
					SecretKey:  "secret",
					AllowUsers: []string{"alice", "bob"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "stcp-with-allowusers"`,
		`type = "stcp"`,
		`allowUsers = ["alice", "bob"]`,
	},
},
{
	name: "STCP upstream - with allowUsers wildcard",
	config: models.Config{
		Common: basicCommon(),
		Upstreams: []models.Upstream{
			{
				Name: "stcp-allow-all",
				Type: 3,
				STCP: models.Upstream_STCP{
					Host:       "127.0.0.1",
					Port:       22,
					SecretKey:  "secret",
					AllowUsers: []string{"*"},
				},
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`name = "stcp-allow-all"`,
		`allowUsers = ["*"]`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add allowUsers to STCP template**

Modify the STCP template block in `pkg/client/utils/template.go` to add after `secretKey`:

```go
{{ if $upstream.STCP.AllowUsers }}
allowUsers = [{{ range $i, $u := $upstream.STCP.AllowUsers }}{{ if $i }}, {{ end }}"{{ $u }}"{{ end }}]
{{ end }}
```

**Step 5: Add allowUsers to XTCP template**

Same for XTCP block.

**Step 6: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 7: Update NewConfig for allowUsers**

Add to STCP handling in `pkg/client/models/config.go`:

```go
if len(upstreamObject.Spec.STCP.AllowUsers) > 0 {
	upstream.STCP.AllowUsers = upstreamObject.Spec.STCP.AllowUsers
}
```

Add same for XTCP.

**Step 8: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add allowUsers support for STCP/XTCP upstreams"
```

---

## Task 3: Add TLS Configuration Types

**Files:**
- Modify: `api/v1alpha1/client_types.go`
- Modify: `api/v1alpha1/types.go`

**Step 1: Add ConfigMapRef and ConfigMapOrSecretRef types**

Add to `api/v1alpha1/types.go`:

```go
type ConfigMapRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ConfigMapOrSecretRef struct {
	// +optional
	Secret *Secret `json:"secret,omitempty"`
	// +optional
	ConfigMap *ConfigMapRef `json:"configMap,omitempty"`
}
```

**Step 2: Add TLS configuration to ClientSpec_Server**

Add to `api/v1alpha1/client_types.go`:

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
}

type ClientSpec_Server_TLS struct {
	// +kubebuilder:default=true
	Enable bool `json:"enable"`
	// +optional
	CertFile *SecretRef `json:"certFile,omitempty"`
	// +optional
	KeyFile *SecretRef `json:"keyFile,omitempty"`
	// +optional
	TrustedCAFile *ConfigMapOrSecretRef `json:"trustedCaFile,omitempty"`
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/client_types.go api/v1alpha1/types.go config/crd/
git commit -m "feat(api): add TLS configuration to Client"
```

---

## Task 4: Add TLS to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Add TLS to Common model**

Add to `Common` struct in `pkg/client/models/config.go`:

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
}

type TLSConfig struct {
	Enable        bool
	CertFile      string
	KeyFile       string
	TrustedCAFile string
}
```

**Step 2: Write failing test for TLS**

Add to test file:

```go
{
	name: "common config with TLS enabled",
	config: models.Config{
		Common: models.Common{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			TLS: &models.TLSConfig{
				Enable: true,
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`transport.tls.enable = true`,
	},
},
{
	name: "common config with TLS and certificates",
	config: models.Config{
		Common: models.Common{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
			TLS: &models.TLSConfig{
				Enable:        true,
				CertFile:      "/etc/frp/tls/tls.crt",
				KeyFile:       "/etc/frp/tls/tls.key",
				TrustedCAFile: "/etc/frp/tls/ca.crt",
			},
		},
	},
	wantErr: false,
	wantContains: []string{
		`transport.tls.enable = true`,
		`transport.tls.certFile = "/etc/frp/tls/tls.crt"`,
		`transport.tls.keyFile = "/etc/frp/tls/tls.key"`,
		`transport.tls.trustedCaFile = "/etc/frp/tls/ca.crt"`,
	},
},
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 4: Add TLS to template**

Add after STUN server in template:

```go
{{ if .Common.TLS }}
transport.tls.enable = {{ .Common.TLS.Enable }}
{{ if .Common.TLS.CertFile }}
transport.tls.certFile = "{{ .Common.TLS.CertFile }}"
{{ end }}
{{ if .Common.TLS.KeyFile }}
transport.tls.keyFile = "{{ .Common.TLS.KeyFile }}"
{{ end }}
{{ if .Common.TLS.TrustedCAFile }}
transport.tls.trustedCaFile = "{{ .Common.TLS.TrustedCAFile }}"
{{ end }}
{{ end }}
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add TLS configuration support"
```

---

## Task 5: Update NewConfig for TLS

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/builder/pod_builder.go`

**Step 1: Add TLS handling to NewConfig**

Add after STUNServer handling in `NewConfig`:

```go
if clientObject.Spec.Server.TLS != nil {
	config.Common.TLS = &TLSConfig{
		Enable: clientObject.Spec.Server.TLS.Enable,
	}

	// Fetch cert from secret
	if clientObject.Spec.Server.TLS.CertFile != nil {
		config.Common.TLS.CertFile = "/etc/frp/tls/tls.crt"
	}

	// Fetch key from secret
	if clientObject.Spec.Server.TLS.KeyFile != nil {
		config.Common.TLS.KeyFile = "/etc/frp/tls/tls.key"
	}

	// Fetch CA from secret or configmap
	if clientObject.Spec.Server.TLS.TrustedCAFile != nil {
		config.Common.TLS.TrustedCAFile = "/etc/frp/tls/ca.crt"
	}
}
```

**Step 2: Update PodBuilder to mount TLS secrets**

This requires more significant changes to pod_builder.go. Add TLS volume mounts:

```go
// In PodBuilder struct
TLSSecret    string
TLSConfigMap string

// In Build method, add volumes if TLS is configured
if n.TLSSecret != "" {
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "tls-certs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: n.TLSSecret,
			},
		},
	})
	pod.Spec.Containers[0].VolumeMounts = append(
		pod.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "tls-certs",
			MountPath: "/etc/frp/tls",
			ReadOnly:  true,
		},
	)
}
```

**Step 3: Commit**

```bash
git add pkg/client/models/config.go pkg/client/builder/pod_builder.go
git commit -m "feat: handle TLS secrets in NewConfig and PodBuilder"
```

---

## Task 6: Add OIDC Authentication Types

**Files:**
- Modify: `api/v1alpha1/client_types.go`

**Step 1: Modify Authentication to support OIDC**

Update `ClientSpec_Server_Authentication`:

```go
type ClientSpec_Server_Authentication struct {
	// +optional
	Token *ClientSpec_Server_Authentication_Token `json:"token,omitempty"`
	// +optional
	OIDC *ClientSpec_Server_Authentication_OIDC `json:"oidc,omitempty"`
}

type ClientSpec_Server_Authentication_OIDC struct {
	ClientID         SecretRef `json:"clientId"`
	ClientSecret     SecretRef `json:"clientSecret"`
	TokenEndpointURL string    `json:"tokenEndpointUrl"`
	// +optional
	Audience string `json:"audience,omitempty"`
	// +optional
	Scope string `json:"scope,omitempty"`
}
```

**Step 2: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 3: Commit**

```bash
git add api/v1alpha1/client_types.go config/crd/
git commit -m "feat(api): add OIDC authentication option"
```

---

## Task 7: Add OIDC to Models and Template

**Files:**
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

**Step 1: Update ServerAuthenticationType constants**

```go
const (
	TokenAuth ServerAuthenticationType = iota
	OIDCAuth  ServerAuthenticationType = iota
)
```

**Step 2: Update ServerAuthentication struct**

```go
type ServerAuthentication struct {
	Type             ServerAuthenticationType
	Token            string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCTokenURL     string
	OIDCAudience     string
	OIDCScope        string
}
```

**Step 3: Write failing test for OIDC**

```go
{
	name: "common config with OIDC authentication",
	config: models.Config{
		Common: models.Common{
			ServerAddress: "frp.example.com",
			ServerPort:    7000,
			ServerAuthentication: models.ServerAuthentication{
				Type:             2, // OIDC
				OIDCClientID:     "my-client-id",
				OIDCClientSecret: "my-client-secret",
				OIDCTokenURL:     "https://auth.example.com/oauth/token",
				OIDCAudience:     "frp-server",
				OIDCScope:        "openid profile",
			},
			AdminAddress:  "0.0.0.0",
			AdminPort:     7400,
			AdminUsername: "admin",
			AdminPassword: "secret",
		},
	},
	wantErr: false,
	wantContains: []string{
		`auth.method = "oidc"`,
		`auth.oidc.clientID = "my-client-id"`,
		`auth.oidc.clientSecret = "my-client-secret"`,
		`auth.oidc.tokenEndpointURL = "https://auth.example.com/oauth/token"`,
		`auth.oidc.audience = "frp-server"`,
		`auth.oidc.scope = "openid profile"`,
	},
	wantNotContain: []string{
		`auth.token`,
	},
},
```

**Step 4: Run test to verify it fails**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: FAIL

**Step 5: Add OIDC to template**

Update authentication section in template:

```go
{{ if eq .Common.ServerAuthentication.Type 1 }}
auth.method = "token"
auth.token = "{{ .Common.ServerAuthentication.Token }}"
{{ end }}

{{ if eq .Common.ServerAuthentication.Type 2 }}
auth.method = "oidc"
auth.oidc.clientID = "{{ .Common.ServerAuthentication.OIDCClientID }}"
auth.oidc.clientSecret = "{{ .Common.ServerAuthentication.OIDCClientSecret }}"
auth.oidc.tokenEndpointURL = "{{ .Common.ServerAuthentication.OIDCTokenURL }}"
{{ if .Common.ServerAuthentication.OIDCAudience }}
auth.oidc.audience = "{{ .Common.ServerAuthentication.OIDCAudience }}"
{{ end }}
{{ if .Common.ServerAuthentication.OIDCScope }}
auth.oidc.scope = "{{ .Common.ServerAuthentication.OIDCScope }}"
{{ end }}
{{ end }}
```

**Step 6: Run test to verify it passes**

Run: `go test ./pkg/client/builder/... -run TestConfigurationBuilder_Build -v`
Expected: PASS

**Step 7: Commit**

```bash
git add pkg/client/models/config.go pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: add OIDC authentication support"
```

---

## Task 8: Update NewConfig for OIDC

**Files:**
- Modify: `pkg/client/models/config.go`

**Step 1: Add OIDC handling to NewConfig**

Replace the token handling with:

```go
if clientObject.Spec.Server.Authentication.Token != nil {
	config.Common.ServerAuthentication.Type = 1

	secret := &corev1.Secret{}
	err := k8sclient.Get(context.TODO(), types.NamespacedName{
		Name:      clientObject.Spec.Server.Authentication.Token.Secret.Name,
		Namespace: clientObject.Namespace,
	}, secret)
	if err != nil {
		return config, err
	}

	tokenByte, ok := secret.Data[clientObject.Spec.Server.Authentication.Token.Secret.Key]
	if !ok {
		return config, errors.NewBadRequest("token key not found in secret")
	}

	config.Common.ServerAuthentication.Token = string(tokenByte)
}

if clientObject.Spec.Server.Authentication.OIDC != nil {
	config.Common.ServerAuthentication.Type = 2

	// Fetch client ID
	secret := &corev1.Secret{}
	err := k8sclient.Get(context.TODO(), types.NamespacedName{
		Name:      clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Name,
		Namespace: clientObject.Namespace,
	}, secret)
	if err != nil {
		return config, err
	}
	clientIDByte, ok := secret.Data[clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Key]
	if !ok {
		return config, errors.NewBadRequest("clientId key not found in secret")
	}
	config.Common.ServerAuthentication.OIDCClientID = string(clientIDByte)

	// Fetch client secret
	err = k8sclient.Get(context.TODO(), types.NamespacedName{
		Name:      clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Name,
		Namespace: clientObject.Namespace,
	}, secret)
	if err != nil {
		return config, err
	}
	clientSecretByte, ok := secret.Data[clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Key]
	if !ok {
		return config, errors.NewBadRequest("clientSecret key not found in secret")
	}
	config.Common.ServerAuthentication.OIDCClientSecret = string(clientSecretByte)

	config.Common.ServerAuthentication.OIDCTokenURL = clientObject.Spec.Server.Authentication.OIDC.TokenEndpointURL
	config.Common.ServerAuthentication.OIDCAudience = clientObject.Spec.Server.Authentication.OIDC.Audience
	config.Common.ServerAuthentication.OIDCScope = clientObject.Spec.Server.Authentication.OIDC.Scope
}
```

**Step 2: Add validation for authentication**

```go
if clientObject.Spec.Server.Authentication.Token == nil && clientObject.Spec.Server.Authentication.OIDC == nil {
	return config, errors.NewBadRequest("either token or oidc authentication is required")
}

if clientObject.Spec.Server.Authentication.Token != nil && clientObject.Spec.Server.Authentication.OIDC != nil {
	return config, errors.NewBadRequest("only one authentication method (token or oidc) can be specified")
}
```

**Step 3: Run all tests**

Run: `make test`
Expected: All PASS

**Step 4: Commit**

```bash
git add pkg/client/models/config.go
git commit -m "feat: handle OIDC authentication in NewConfig"
```

---

## Task 9: Add Example Manifests

**Files:**
- Create: `examples/secure-access/`

**Step 1: Create examples directory**

Run: `mkdir -p examples/secure-access`

**Step 2: Create STCP with allowUsers example**

Create `examples/secure-access/stcp-allowusers.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: private-api
spec:
  client: secure-client
  stcp:
    host: api-service.default.svc
    port: 8080
    secretKey:
      secret:
        name: stcp-secret
        key: secretKey
    allowUsers:
      - "alice"
      - "bob"
```

**Step 3: Create OIDC client example**

Create `examples/secure-access/client-oidc.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: oidc-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      oidc:
        clientId:
          secret:
            name: oidc-creds
            key: clientId
        clientSecret:
          secret:
            name: oidc-creds
            key: clientSecret
        tokenEndpointUrl: "https://auth.example.com/oauth/token"
        audience: "frp-server"
        scope: "openid profile"
```

**Step 4: Create TLS client example**

Create `examples/secure-access/client-tls.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: tls-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    tls:
      enable: true
      certFile:
        secret:
          name: tls-certs
          key: tls.crt
      keyFile:
        secret:
          name: tls-certs
          key: tls.key
      trustedCaFile:
        secret:
          name: tls-certs
          key: ca.crt
```

**Step 5: Commit**

```bash
git add examples/secure-access/
git commit -m "docs(examples): add secure access examples"
```

---

## Task 10: Update Helm Chart and Final Test

**Step 1: Copy CRDs to Helm chart**

Run: `cp config/crd/bases/*.yaml charts/frp-operator/crds/`

**Step 2: Run all tests**

Run: `make test`
Expected: All PASS

**Step 3: Build**

Run: `make build`
Expected: No errors

**Step 4: Commit**

```bash
git add charts/frp-operator/crds/
git commit -m "feat: complete Phase 2 - secure access control"
```

---

## Summary

Phase 2 adds security features:
- **allowUsers**: Control which FRP users can connect to STCP/XTCP tunnels
- **OIDC Authentication**: Alternative to token auth for enterprise SSO
- **TLS Configuration**: Client certificates and CA verification

Files modified:
- `api/v1alpha1/client_types.go` - TLS, OIDC types
- `api/v1alpha1/upstream_types.go` - allowUsers field
- `api/v1alpha1/types.go` - ConfigMapRef, SecretRef
- `pkg/client/models/config.go` - Model structs, NewConfig
- `pkg/client/utils/template.go` - TOML templates
- `pkg/client/builder/pod_builder.go` - TLS volume mounts
- `examples/secure-access/` - Example manifests
