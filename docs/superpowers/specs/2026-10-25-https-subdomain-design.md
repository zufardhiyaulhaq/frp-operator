{% raw %}
# `subdomain` on HTTPS and TCPMUX Upstreams — design

Agreed with the owner in a brainstorming and grilling session on 2026-10-25 (Q1–Q8). Fixes
[issue #16](https://github.com/zufardhiyaulhaq/frp-operator/issues/16). This is the source of truth
for the implementation plan.

## Why

frp names an `http`, `https` or `tcpmux` proxy in one of two ways (frp's `DomainConfig`):

- `customDomains`: full hostnames the Upstream's author owns (`zufardhiyaulhaq.com`). DNS for each
  name points at frps.
- `subdomain`: one label; frps appends its `subDomainHost` (`pay` → `pay.foo.bar`). The frps
  operator owns `foo.bar` and sets up one wildcard DNS record `*.foo.bar`.

frps rejects any `customDomains` entry under its `subDomainHost` ("custom domain [x] should not
belong to subdomain host [y]"), so names under `subDomainHost` can only be claimed with `subdomain`.
The operator's `Upstream` has `subdomain` on `spec.http` only. `spec.https` and `spec.tcpmux` have
only `customDomains`, and the CRD requires it. So an HTTPS or TCPMUX Upstream can never get a name
under an frps's `subDomainHost` — including our own `Server` with `vhost.subdomainHost`. The reporter
had to bypass the operator with a hand-written frpc ConfigMap.

TLS is not involved: frps never terminates TLS for websites. `http` is plain HTTP end to end;
`https` is SNI passthrough, and the backend Service holds the certificate. `customDomains` vs
`subdomain` only decides the hostname.

## Goals

- `spec.https.subdomain` and `spec.tcpmux.subdomain` (string, optional), rendered as
  `subdomain = "<label>"`, like `spec.http.subdomain` today (Q1).
- `customDomains` becomes optional on `spec.https` and `spec.tcpmux` (only relaxes the CRD; every
  existing Upstream stays valid).
- On `http`, `https` and `tcpmux`: `customDomains` **or** `subdomain` **or both** must be set;
  otherwise `ConfigSynced=False/InvalidConfig` and a Warning event on the Client (Q2, Q4, Q5).
- `subdomain` is one label: the CRD rejects a value containing `.` or `*` at `kubectl apply`, on
  `http`, `https` and `tcpmux` (Q3).
- Docs explain `customDomains` vs `subdomain`, the "names under `subDomainHost` must use `subdomain`"
  rule, and that TLS ends at the backend.

## Non-goals

- Checking `customDomains` against the frps's `subDomainHost`: the frpc side does not know it; frps
  reports that error itself (Q6).
- TLS termination in frpc (`https2http` plugin on an `https` proxy) — a separate gap.
- A single `domains:` field that splits names automatically (it would need the frps's
  `subDomainHost`).

## Anti-goals

- Breaking any existing `Upstream`.
- A rule of "exactly one of the two": frp allows both (one proxy answering on `pay.foo.bar` and
  `pay.mycompany.com`).

## Decisions

| # | Decision |
|---|---|
| Q1 | Fix `https` and `tcpmux` together |
| Q2 | The "`customDomains` or `subdomain` (or both)" rule applies to `http`, `https` and `tcpmux` |
| Q3 | CRD pattern on `subdomain` (all three types) rejecting `.` and `*` |
| Q4 | The "or" rule is an operator check (`InvalidConfig`), not a CRD CEL rule — it also covers clusters with an older CRD |
| Q5 | Both fields together are allowed |
| Q6 | No check against frps's `subDomainHost` |
| Q7 | e2e proves it: a `Server` with `vhost.subdomainHost`, an `http` and an `https` Upstream using `subdomain`, reached with `curl --resolve` through frps |
| Q8 | Ships on its own as **v0.15.0** (new CRD field), PR says "Fixes #16" |
| B1 | *Builder's call:* the "or" rule skips Upstreams with `enabled: false`, matching frpc v0.71, which drops disabled proxies before validating (`cmd/frpc/sub/root.go:170-174`, `pkg/config/load.go:418`). Otherwise a paused Upstream would fail its whole Client |
| B2 | *Builder's call:* the comment on `ServerSpec_Vhost.SubdomainHost` also names TCPMUX Upstreams |

## Verified facts (frp v0.71.1 source)

- `HTTPProxyConfig`, `HTTPSProxyConfig` and `TCPMuxProxyConfig` all embed
  `DomainConfig { customDomains, subdomain }` (`pkg/config/v1/proxy.go`).
- frpc rejects a proxy with neither (`subdomain and custom domains should not be both empty`).
- frps rejects `customDomains` under `subDomainHost`, `subdomain` without `subDomainHost`, and a
  `subdomain` containing `.` or `*` (`pkg/config/v1/validation/proxy.go:74-100`).

## Testing

- Unit: model maps `subdomain` for https/tcpmux; template renders it; the "or" rule rejects neither
  set (http, https, tcpmux) and accepts each alone and both.
- Real binary: `make test-frpc-config` cases for https and tcpmux with `subdomain`.
- e2e (Q7): as above.

## Left to the builder

- The exact CRD pattern and its error message.
- Where the "or" check sits in `models.NewConfig`.
- Which example gets a `subdomain` HTTPS Upstream.
{% endraw %}
