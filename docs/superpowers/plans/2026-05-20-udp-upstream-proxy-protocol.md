{% raw %}
# UDP Upstream Proxy Protocol + Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional `proxyProtocol` and `transport` fields to the UDP `Upstream`, reaching transport parity with TCP, using dedicated UDP structs.

**Architecture:** Mirror the existing TCP code path across four layers — CRD type (`api/v1alpha1`), internal model (`pkg/client/models`), CR→model transform (`NewConfig`), and the frpc TOML template (`pkg/client/utils`). New `UpstreamSpec_UDP_Transport` / `Upstream_UDP_Transport` structs are created; no TCP types are referenced. The change is additive and non-breaking.

**Tech Stack:** Go 1.23 · Kubebuilder v3 / controller-gen · controller-runtime v0.18.4 · Go `text/template`

**Spec:** `docs/superpowers/specs/2026-05-20-udp-upstream-proxy-protocol-design.md`

---

## File Structure

**Modified:**
- `api/v1alpha1/upstream_types.go` — two new structs + two new fields on `UpstreamSpec_UDP`
- `api/v1alpha1/zz_generated.deepcopy.go` — regenerated (do not hand-edit)
- `charts/frp-operator/crds/frp.zufardhiyaulhaq.com_upstreams.yaml`, `charts/frp-operator/crds/crds.yaml`, `config/crd/bases/frp.zufardhiyaulhaq.com_upstreams.yaml` — regenerated
- `pkg/client/models/config.go` — two new model structs + two new fields on `Upstream_UDP` + transform logic
- `pkg/client/utils/template.go` — UDP block gains proxyProtocol + transport rendering
- `pkg/client/models/config_test.go` — new transform tests
- `pkg/client/builder/configuration_builder_test.go` — new template/render tests

**Created:**
- `examples/advanced/udp-transport.yaml` — usage example

---

## Task 1: Add UDP transport CRD types

**Files:**
- Modify: `api/v1alpha1/upstream_types.go`
- Regenerated: `api/v1alpha1/zz_generated.deepcopy.go`, `charts/frp-operator/crds/*.yaml`, `config/crd/bases/frp.zufardhiyaulhaq.com_upstreams.yaml`

- [ ] **Step 1: Add the two new transport structs**

In `api/v1alpha1/upstream_types.go`, find the `UpstreamSpec_UDP_Server` struct:

```go
type UpstreamSpec_UDP_Server struct {
	Port int `json:"port"`
}
```

Immediately after it, add:

```go
type UpstreamSpec_UDP_Transport struct {
	// +kubebuilder:default=true
	UseEncryption bool `json:"useEncryption"`
	// +kubebuilder:default=false
	UseCompression bool `json:"useCompression"`
	// +optional
	BandwidthLimit *UpstreamSpec_UDP_Transport_BandwidthLimit `json:"bandwidthLimit"`
}

type UpstreamSpec_UDP_Transport_BandwidthLimit struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
	// +kubebuilder:validation:Enum=KB;MB
	Type string `json:"type"`
}
```

- [ ] **Step 2: Add the two new fields to `UpstreamSpec_UDP`**

In the same file, find:

```go
type UpstreamSpec_UDP struct {
	Host   string                  `json:"host"`
	Port   int                     `json:"port"`
	Server UpstreamSpec_UDP_Server `json:"server"`
}
```

Replace it with:

```go
type UpstreamSpec_UDP struct {
	Host   string                  `json:"host"`
	Port   int                     `json:"port"`
	Server UpstreamSpec_UDP_Server `json:"server"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol,omitempty"`
	// +optional
	Transport *UpstreamSpec_UDP_Transport `json:"transport,omitempty"`
}
```

- [ ] **Step 3: Regenerate DeepCopy code and CRD manifests**

Run: `make generate && make manifests`
Expected: both commands exit 0. `api/v1alpha1/zz_generated.deepcopy.go` now contains `DeepCopyInto`/`DeepCopy` for `UpstreamSpec_UDP_Transport` and `UpstreamSpec_UDP_Transport_BandwidthLimit`, and `UpstreamSpec_UDP`'s `DeepCopyInto` copies the new pointer fields.

- [ ] **Step 4: Verify the CRD YAML contains the new fields**

Run: `grep -A3 'proxyProtocol' charts/frp-operator/crds/frp.zufardhiyaulhaq.com_upstreams.yaml | head -20`
Expected: a `proxyProtocol` property with `enum: [v1, v2]` appears under the `udp` schema (it also appears under `tcp`/`stcp`/`xtcp`/`https` — that is fine).

Run: `grep -c 'bandwidthLimit' charts/frp-operator/crds/frp.zufardhiyaulhaq.com_upstreams.yaml`
Expected: a count of `2` or more (TCP already had one; UDP adds another).

- [ ] **Step 5: Verify the project still builds**

Run: `make build`
Expected: compiles `bin/manager` with no errors.

- [ ] **Step 6: Commit**

```bash
git add api/v1alpha1/upstream_types.go api/v1alpha1/zz_generated.deepcopy.go charts/frp-operator/crds/ config/crd/bases/
git commit -m "feat: add proxyProtocol and transport fields to UDP Upstream CRD"
```

---

## Task 2: Add UDP model structs and transform logic

**Files:**
- Modify: `pkg/client/models/config.go`
- Test: `pkg/client/models/config_test.go`

- [ ] **Step 1: Write the failing transform tests**

In `pkg/client/models/config_test.go`, add these two tests immediately after the existing `TestNewConfig_UDPUpstream` function (which ends at the line `}` following the `ServerPort` assertion):

```go
func TestNewConfig_UDPUpstreamWithProxyProtocol(t *testing.T) {
	fakeClient := createFakeClient(createDefaultTokenSecret("default")).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	proxyProtocol := "v2"
	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "udp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				UDP: &frpv1alpha1.UpstreamSpec_UDP{
					Host:          "127.0.0.1",
					Port:          53,
					Server:        frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
					ProxyProtocol: &proxyProtocol,
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	upstream := config.Upstreams[0]
	if upstream.UDP.ProxyProtocol == nil {
		t.Fatalf("NewConfig() upstream.UDP.ProxyProtocol = nil, want %q", "v2")
	}
	if *upstream.UDP.ProxyProtocol != "v2" {
		t.Errorf("NewConfig() upstream.UDP.ProxyProtocol = %v, want %v", *upstream.UDP.ProxyProtocol, "v2")
	}
}

func TestNewConfig_UDPUpstreamWithTransport(t *testing.T) {
	fakeClient := createFakeClient(createDefaultTokenSecret("default")).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "udp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				UDP: &frpv1alpha1.UpstreamSpec_UDP{
					Host:   "127.0.0.1",
					Port:   53,
					Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
					Transport: &frpv1alpha1.UpstreamSpec_UDP_Transport{
						UseEncryption:  true,
						UseCompression: true,
						BandwidthLimit: &frpv1alpha1.UpstreamSpec_UDP_Transport_BandwidthLimit{
							Enabled: true,
							Limit:   100,
							Type:    "MB",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	upstream := config.Upstreams[0]
	if upstream.UDP.Transport == nil {
		t.Fatalf("NewConfig() upstream.UDP.Transport = nil, want non-nil")
	}
	if !upstream.UDP.Transport.UseEncryption {
		t.Errorf("NewConfig() upstream.UDP.Transport.UseEncryption = false, want true")
	}
	if !upstream.UDP.Transport.UseCompression {
		t.Errorf("NewConfig() upstream.UDP.Transport.UseCompression = false, want true")
	}
	if upstream.UDP.Transport.BandwidthLimit == nil {
		t.Fatalf("NewConfig() upstream.UDP.Transport.BandwidthLimit = nil, want non-nil")
	}
	if !upstream.UDP.Transport.BandwidthLimit.Enabled {
		t.Errorf("NewConfig() BandwidthLimit.Enabled = false, want true")
	}
	if upstream.UDP.Transport.BandwidthLimit.Limit != 100 {
		t.Errorf("NewConfig() BandwidthLimit.Limit = %v, want 100", upstream.UDP.Transport.BandwidthLimit.Limit)
	}
	if upstream.UDP.Transport.BandwidthLimit.Type != "MB" {
		t.Errorf("NewConfig() BandwidthLimit.Type = %v, want MB", upstream.UDP.Transport.BandwidthLimit.Type)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/client/models/ -run 'TestNewConfig_UDPUpstreamWith' -v`
Expected: FAIL — the package fails to **compile**. The `frpv1alpha1.UpstreamSpec_UDP_Transport` / `UpstreamSpec_UDP_Transport_BandwidthLimit` types referenced in the test input already exist (added in Task 1), but the test also reads `upstream.UDP.ProxyProtocol` and `upstream.UDP.Transport` — model fields on `Upstream_UDP` that do not exist yet. They are added in Step 3.

- [ ] **Step 3: Add the model structs and transform logic**

In `pkg/client/models/config.go`, find the `Upstream_UDP` struct:

```go
type Upstream_UDP struct {
	Host       string
	Port       int
	ServerPort int
}
```

Replace it with:

```go
type Upstream_UDP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	Transport     *Upstream_UDP_Transport
}

type Upstream_UDP_Transport struct {
	UseCompression bool
	UseEncryption  bool
	BandwidthLimit *Upstream_UDP_Transport_BandwidthLimit
}

type Upstream_UDP_Transport_BandwidthLimit struct {
	Enabled bool
	Limit   int
	Type    string
}
```

Then find the UDP transform block:

```go
		if upstreamObject.Spec.UDP != nil {
			upstream.Type = 2
			upstream.UDP.Host = upstreamObject.Spec.UDP.Host
			upstream.UDP.Port = upstreamObject.Spec.UDP.Port
			upstream.UDP.ServerPort = upstreamObject.Spec.UDP.Server.Port
		}
```

Replace it with:

```go
		if upstreamObject.Spec.UDP != nil {
			upstream.Type = 2
			upstream.UDP.Host = upstreamObject.Spec.UDP.Host
			upstream.UDP.Port = upstreamObject.Spec.UDP.Port
			upstream.UDP.ServerPort = upstreamObject.Spec.UDP.Server.Port

			if upstreamObject.Spec.UDP.ProxyProtocol != nil {
				upstream.UDP.ProxyProtocol = upstreamObject.Spec.UDP.ProxyProtocol
			}

			if upstreamObject.Spec.UDP.Transport != nil {
				upstream.UDP.Transport = &Upstream_UDP_Transport{
					UseCompression: upstreamObject.Spec.UDP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.UDP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.UDP.Transport.BandwidthLimit != nil {
					upstream.UDP.Transport.BandwidthLimit = &Upstream_UDP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.UDP.Transport.BandwidthLimit.Enabled,
						Limit:   upstreamObject.Spec.UDP.Transport.BandwidthLimit.Limit,
						Type:    upstreamObject.Spec.UDP.Transport.BandwidthLimit.Type,
					}
				}
			}
		}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/client/models/ -run 'TestNewConfig_UDPUpstream' -v`
Expected: PASS — `TestNewConfig_UDPUpstream`, `TestNewConfig_UDPUpstreamWithProxyProtocol`, and `TestNewConfig_UDPUpstreamWithTransport` all pass.

- [ ] **Step 5: Run the full models package test suite (regression guard)**

Run: `go test ./pkg/client/models/ -v`
Expected: PASS — all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/client/models/config.go pkg/client/models/config_test.go
git commit -m "feat: map UDP proxyProtocol and transport in NewConfig"
```

---

## Task 3: Render proxyProtocol and transport in the UDP template block

**Files:**
- Modify: `pkg/client/utils/template.go`
- Test: `pkg/client/builder/configuration_builder_test.go`

- [ ] **Step 1: Write the failing render tests**

In `pkg/client/builder/configuration_builder_test.go`, find the existing `"UDP upstream"` test case (the table entry whose `name` field is `"UDP upstream"`). Immediately after that case's closing `},`, insert these three new table cases:

```go
		{
			name: "UDP upstream with proxy protocol",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "udp-with-proxy-protocol",
						Type: 2,
						UDP: models.Upstream_UDP{
							Host:          "127.0.0.1",
							Port:          53,
							ServerPort:    5353,
							ProxyProtocol: stringPtr("v2"),
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "udp-with-proxy-protocol"`,
				`type = "udp"`,
				`transport.proxyProtocolVersion = "v2"`,
			},
		},
		{
			name: "UDP upstream with transport",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "udp-with-transport",
						Type: 2,
						UDP: models.Upstream_UDP{
							Host:       "127.0.0.1",
							Port:       53,
							ServerPort: 5353,
							Transport: &models.Upstream_UDP_Transport{
								UseEncryption:  true,
								UseCompression: true,
								BandwidthLimit: &models.Upstream_UDP_Transport_BandwidthLimit{
									Enabled: true,
									Limit:   100,
									Type:    "MB",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "udp-with-transport"`,
				`type = "udp"`,
				`transport.useEncryption = true`,
				`transport.useCompression = true`,
				`transport.bandwidthLimit = "100MB"`,
				`transport.bandwidthLimitMode = "client"`,
			},
		},
		{
			name: "UDP upstream without transport renders no transport lines",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "udp-plain",
						Type: 2,
						UDP: models.Upstream_UDP{
							Host:       "127.0.0.1",
							Port:       53,
							ServerPort: 5353,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "udp-plain"`,
				`type = "udp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 53`,
				`remotePort = 5353`,
			},
			wantNotContain: []string{
				`transport.proxyProtocolVersion`,
				`transport.useEncryption`,
				`transport.useCompression`,
				`transport.bandwidthLimit`,
			},
		},
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/client/builder/ -run TestConfigurationBuilder_Build -v`
Expected: FAIL — the `"UDP upstream with proxy protocol"` and `"UDP upstream with transport"` cases fail because the rendered TOML does not contain `transport.proxyProtocolVersion` / `transport.useEncryption` etc. (the UDP template block does not render them yet). The `"without transport"` case may already pass.

- [ ] **Step 3: Extend the UDP block in the template**

In `pkg/client/utils/template.go`, find the UDP block:

```
{{ if eq $upstream.Type 2 }}
name = "{{ $upstream.Name }}"
type = "udp"
localIP = "{{ $upstream.UDP.Host }}"
localPort = {{ $upstream.UDP.Port }}
remotePort = {{ $upstream.UDP.ServerPort }}
{{ end }}
```

Replace it with:

```
{{ if eq $upstream.Type 2 }}
name = "{{ $upstream.Name }}"
type = "udp"
localIP = "{{ $upstream.UDP.Host }}"
localPort = {{ $upstream.UDP.Port }}
remotePort = {{ $upstream.UDP.ServerPort }}

{{ if $upstream.UDP.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.UDP.ProxyProtocol }}"
{{ end }}

{{ if $upstream.UDP.Transport }}
transport.useEncryption = {{ $upstream.UDP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.UDP.Transport.UseCompression }}
{{ if $upstream.UDP.Transport.BandwidthLimit }}
{{ if $upstream.UDP.Transport.BandwidthLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.UDP.Transport.BandwidthLimit.Limit }}{{ $upstream.UDP.Transport.BandwidthLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ end }}
{{ end }}
```

Note: `$upstream.UDP.ProxyProtocol` is a `*string`. In Go templates, dereferencing `{{ $upstream.UDP.ProxyProtocol }}` on a non-nil string pointer prints the pointed-to value — this is the exact same construct the TCP block at `transport.proxyProtocolVersion = "{{ $upstream.TCP.ProxyProtocol }}"` already uses, so it is known to work. The builder strips blank lines after rendering (`TestConfigurationBuilder_Build_RemovesEmptyLines`), so the extra blank lines in the template are harmless.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/client/builder/ -run TestConfigurationBuilder_Build -v`
Expected: PASS — all table cases pass, including the three new UDP cases and the pre-existing `"UDP upstream"` case.

- [ ] **Step 5: Run the full builder package test suite (regression guard)**

Run: `go test ./pkg/client/builder/ -v`
Expected: PASS — all tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/client/utils/template.go pkg/client/builder/configuration_builder_test.go
git commit -m "feat: render proxyProtocol and transport for UDP upstreams"
```

---

## Task 4: Add the UDP example file

**Files:**
- Create: `examples/advanced/udp-transport.yaml`

- [ ] **Step 1: Create the example file**

Create `examples/advanced/udp-transport.yaml` with exactly this content:

```yaml
# UDP Upstream with Proxy Protocol and transport tuning
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: dns
  namespace: default
spec:
  client: client-01
  udp:
    host: dns-service.default.svc.cluster.local
    port: 53
    server:
      port: 6002
    proxyProtocol: v2
    transport:
      useEncryption: true
      useCompression: true
      bandwidthLimit:
        enabled: true
        limit: 1024
        type: MB
```

- [ ] **Step 2: Verify the YAML is well-formed**

Run: `ruby -ryaml -e 'YAML.load_stream(File.read(ARGV[0])); puts "YAML OK"' examples/advanced/udp-transport.yaml`
Expected: prints `YAML OK` (Ruby ships with macOS and its `yaml` library is in the standard library, so no install is needed). If the file has a syntax error, Ruby prints a `Psych::SyntaxError` instead.

- [ ] **Step 3: Verify the example matches the CRD schema**

Run: `make manifests >/dev/null 2>&1; grep -A30 'proxyProtocol' charts/frp-operator/crds/frp.zufardhiyaulhaq.com_upstreams.yaml | grep -E 'useEncryption|useCompression|bandwidthLimit'`
Expected: the `useEncryption`, `useCompression`, and `bandwidthLimit` keys exist in the generated UDP CRD schema, confirming the example's field names are valid.

- [ ] **Step 4: Commit**

```bash
git add examples/advanced/udp-transport.yaml
git commit -m "docs: add UDP upstream proxy protocol example"
```

---

## Task 5: Full verification

**Files:** none modified.

- [ ] **Step 1: Format, vet, build**

Run: `make fmt && make vet && make build`
Expected: all three succeed; `bin/manager` is produced.

- [ ] **Step 2: Full test suite**

Run: `make test`
Expected: PASS — `pkg/client/builder` and `pkg/client/models` packages pass, including all new UDP tests. `output/coverage.html` is regenerated.

- [ ] **Step 3: Confirm generated artifacts are in sync**

Run: `make generate && make manifests && git status --short`
Expected: `git status --short` shows no unexpected modifications — i.e. running codegen again produces no diff beyond what was already committed. If it does show a diff, commit the regenerated files:

```bash
git add api/v1alpha1/zz_generated.deepcopy.go charts/frp-operator/crds/ config/crd/bases/
git commit -m "chore: sync generated CRD artifacts"
```

- [ ] **Step 4: No commit for this task**

Task 5 is verification-only. If any step fails, return to the relevant earlier task, fix, recommit there, and re-run Task 5 from Step 1.

---

## Out of scope (do NOT touch)

- TCP `proxyURL` relocation to the `Client` CRD — separate follow-up spec.
- TCP, STCP, XTCP, HTTP, HTTPS, TCPMUX upstream paths.
- A configurable `bandwidthLimitMode` (stays the literal `"client"` in the template).
- `Chart.yaml` version bump and `make readme` — handled by the normal release flow.
{% endraw %}
