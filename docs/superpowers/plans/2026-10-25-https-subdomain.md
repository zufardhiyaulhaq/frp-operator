{% raw %}
# `subdomain` on HTTPS and TCPMUX Upstreams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `Upstream.spec.https.subdomain` and `spec.tcpmux.subdomain`, optional `customDomains` on both, a CRD check that `subdomain` is one label, and an operator rule that `http`/`https`/`tcpmux` set `customDomains` or `subdomain` (or both). Fixes issue #16.

**Architecture:** API fields plus a CRD pattern; the model maps `subdomain` the way `http` already does; the frpc template renders it; one validation function in `pkg/client/models/config.go` enforces the "or" rule for enabled Upstreams. An e2e case proves it against a real frps with `subDomainHost`.

**Tech Stack:** Go 1.23, Kubebuilder v3, controller-runtime v0.18.4, frp v0.71.1, Docker (`make test-frpc-config`), k3d (e2e, CI only).

**Spec:** `docs/superpowers/specs/2026-10-25-https-subdomain-design.md`

## Global Constraints

- The rule is "`customDomains` **or** `subdomain` **or both**" for `http`, `https`, `tcpmux`; error text: `upstream "<name>": <type> needs customDomains or subdomain`; reported by `NewConfig` as `errors.NewBadRequest` (→ `ConfigSynced=False/InvalidConfig`).
- The rule skips Upstreams with `enabled: false` (frpc drops disabled proxies before validating — spec B1).
- `subdomain` on all three types: `+kubebuilder:validation:Pattern=`^[^.*]+$`` (no `.`, no `*`).
- `customDomains` on `https` and `tcpmux` becomes `json:"customDomains,omitempty"` + `+optional`; `customDomains` and `subdomain` render only when set.
- After `api/v1alpha1` changes: `make generate manifests`, then `cat config/crd/bases/frp.zufardhiyaulhaq.com_{clients,upstreams,visitors,serverpools,servers}.yaml > charts/frp-operator/crds/crds.yaml`.
- `pkg/client/utils/template_test.go` mirror structs (`testUpstreamHTTPS`, `testUpstreamTCPMUX`) gain `Subdomain string`.
- Commits `git commit -s`, trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; docs under `docs/superpowers/` wrapped in a Liquid `raw` block.

## Review Focus

1. **An existing Upstream with only `customDomains`** (https or tcpmux) must render exactly as before — no `subdomain` key, no empty `customDomains = []`. → `TestTemplateHTTPSSubdomain` asserts the customDomains-only case has no `subdomain`.
2. **A paused Upstream (`enabled: false`) with neither field** must not fail its Client. → `TestNewConfig_DomainRule` has that case.
3. **Both fields set** must render both keys. → `TestTemplateHTTPSSubdomain` both-case; `frpc verify` case `https_subdomain_and_domains`.
4. **`subdomain: pay.foo.bar`** must be rejected at apply. → the CRD pattern; checked in Task 1 Step 4 by reading the generated CRD.
5. **An old CRD** (pre-v0.15.0, `customDomains` still required) is unaffected — no test needed; the rule is additive and the operator check runs regardless.

---

### Task 1: API, model and template

**Files:**
- Modify: `api/v1alpha1/upstream_types.go` (`UpstreamSpec_HTTP` ~L103, `UpstreamSpec_HTTPS` ~L141, `UpstreamSpec_TCPMUX` ~L50) (+ generated CRDs)
- Modify: `pkg/client/models/config.go` (`Upstream_HTTPS` ~L326, `Upstream_TCPMUX` ~L154, mapping ~L981 and ~L1000)
- Modify: `pkg/client/utils/template.go` (https block ~L313, tcpmux block ~L343)
- Test: `pkg/client/utils/template_test.go`, `pkg/client/models/config_test.go`, `test/frpcconfig/frpcconfig_test.go`

**Interfaces:**
- Produces: `UpstreamSpec_HTTPS.Subdomain`, `UpstreamSpec_TCPMUX.Subdomain` (`json:"subdomain,omitempty"`); `models.Upstream_HTTPS.Subdomain`, `models.Upstream_TCPMUX.Subdomain` (string).

- [ ] **Step 1: Write the failing tests**
  - `TestTemplateHTTPSSubdomain`: https with `Subdomain: "pay"` only → contains `subdomain = "pay"`, not `customDomains`; with `CustomDomains: ["pay.mycompany.com"]` only → no `subdomain`; with both → both keys.
  - `TestTemplateTCPMUXSubdomain`: tcpmux with `Subdomain: "ssh"` → contains `subdomain = "ssh"`, not `customDomains`.
  - `TestNewConfig_HTTPSAndTCPMUXSubdomain`: Upstreams `{HTTPS: {Subdomain: "pay"}}` and `{TCPMUX: {Subdomain: "ssh", Multiplexer: "httpconnect"}}` → `config.Upstreams[i].HTTPS.Subdomain == "pay"`, `.TCPMUX.Subdomain == "ssh"`.
  - frpc verify cases: `https_subdomain` (`subdomain = "pay"`), `https_subdomain_and_domains` (both), `tcpmux_subdomain` (`subdomain = "ssh"`).

- [ ] **Step 2: Run to verify they fail**
  Run: `go test ./pkg/client/...`
  Expected: build failure, `unknown field Subdomain in struct literal of type v1alpha1.UpstreamSpec_HTTPS`.

- [ ] **Step 3: Implement** — the fields and markers from Global Constraints (pattern on all three `subdomain` fields; `customDomains` optional on https/tcpmux); `make generate manifests` + rebuild `crds.yaml`; model mapping; template renders `subdomain` and `customDomains` only when set, like the http block.

- [ ] **Step 4: Verify**
  Run: `go test ./pkg/client/... ./controllers/` → `ok`.
  Run: `python3 -c "import yaml;p=yaml.safe_load(open('config/crd/bases/frp.zufardhiyaulhaq.com_upstreams.yaml'))['spec']['versions'][0]['schema']['openAPIV3Schema']['properties']['spec']['properties'];print([(k,p[k].get('required'),p[k]['properties']['subdomain'].get('pattern')) for k in ('http','https','tcpmux')])"`
  Expected: `https` required `['host','port']`, `tcpmux` required `['host','multiplexer','port']`, pattern `^[^.*]+$` on all three.
  Run: `make test-frpc-config 2>&1 | grep -E "verified|^ok|FAIL"` → `verified 72 generated configs` (69 + 3), `verified 9 generated server configs`, `ok`.

- [ ] **Step 5: Commit** — `feat(upstream): subdomain on https and tcpmux; customDomains optional (#16)`

---

### Task 2: "customDomains or subdomain" rule

**Files:**
- Modify: `pkg/client/models/config.go` (new func next to `validateUpstreamNames` ~L519; call it in `NewConfig` next to ~L601)
- Test: `pkg/client/models/config_test.go`

**Interfaces:**
- Produces: `func validateUpstreamDomains(upstreamObjects []frpv1alpha1.Upstream) error` — uses existing `isEnabled(*bool) bool`.

- [ ] **Step 1: Write the failing test** — `TestNewConfig_DomainRule`, table over `NewConfig` with one Upstream each:
  - http / https / tcpmux with neither → error containing `needs customDomains or subdomain` (and the Upstream name)
  - each type with only `customDomains`, only `subdomain`, and both → no error
  - https with neither and `Enabled: ptr(false)` → no error

- [ ] **Step 2: Run to verify it fails** — `go test ./pkg/client/models/ -run TestNewConfig_DomainRule` → the "neither" cases report no error.

- [ ] **Step 3: Implement** `validateUpstreamDomains` and call it from `NewConfig` (returns `errors.NewBadRequest`).

- [ ] **Step 4: Verify** — `go test ./pkg/client/... ./controllers/` → `ok` (existing http tests without domains, if any, must be given a domain — the rule is intended).

- [ ] **Step 5: Commit** — `feat(upstream): http, https and tcpmux need customDomains or subdomain`

---

### Task 3: e2e against a Server with `subdomainHost`

**Files:**
- Modify: `test/e2e/server_feature_test.go`

- [ ] **Step 1: Write the case** — `TestServerSubdomain`:
  - `c.Server("public", …)` with `Authentication` → `frps-token`, `Vhost: {HTTPPort: 8080, HTTPSPort: 8443, SubdomainHost: "e2e.test"}`; wait `Ready`.
  - A Client pointed at `status.address` with the `frps-token`; Upstreams `http {Host: httpEchoHost, Port: 80, Subdomain: "web"}` and `https {Host: httpEchoHost, Port: 443, Subdomain: "secure"}`; wait synced and both proxies running.
  - Read the Service `public-frps` ClusterIP; `expectBody` with `--resolve web.e2e.test:8080:<ip> http://web.e2e.test:8080/hello` and `-k --resolve secure.e2e.test:8443:<ip> https://secure.e2e.test:8443/hello`, both expecting `"path": "/hello"`.
- [ ] **Step 2: Verify it compiles** — `go vet -tags e2e ./test/e2e/` → clean. (The suite runs in the PR's CI; not locally unless the owner asks.)
- [ ] **Step 3: Commit** — `test(e2e): http and https subdomain through a Server with subdomainHost`

---

### Task 4: Docs

**Files:**
- Modify: `api/v1alpha1/server_types.go:119` (comment names HTTP, HTTPS and TCPMUX — spec B2; then `make manifests` + rebuild `crds.yaml`), `charts/frp-operator/README.md.gotmpl` (then `make readme`), `examples/server/` (an `https` Upstream with `subdomain`)

- [ ] **Step 1: Write the docs** — README: a short "`customDomains` or `subdomain`" section (who owns the domain, DNS, names under `subdomainHost` must use `subdomain`, TLS ends at the backend for `https`); the Server section points at it; the example adds an `https` Upstream with `subdomain: pay` next to `subdomainHost: example.com`.
- [ ] **Step 2: Verify** — `make readme && make test-frpc-config 2>&1 | grep -E "verified|^ok|FAIL"` → `verified 72 generated configs` (unchanged: the new Upstream joins the existing example Client `to-public-01`, whose rendered config now also holds `subdomain = "pay"`), `verified 9 generated server configs`, `ok`. `make test` → every package `ok`.
- [ ] **Step 3: Commit** — `docs(upstream): customDomains or subdomain (#16)`

---

## After the plan

- PR with "Fixes #16"; e2e runs in CI. Release **v0.15.0 / chart 1.13.0** with the `release-frp-operator` skill.
{% endraw %}
