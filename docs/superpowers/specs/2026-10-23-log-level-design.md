{% raw %}
# Log level for frps and frpc, and frp v0.71.1 — design

Agreed with the owner in a brainstorming session and a grilling session (G1–G9) on 2026-10-23.
This is the source of truth for the implementation plan.

## Why

The owner wants to switch frps or frpc to `debug` while chasing a tunnel problem, and turn logs
back down (`info`, `warn`, `error`) when they are noisy. Today the operator writes no `log.*` keys,
so frps and frpc always run at frp's defaults (stdout, `info`) and there is no way to change that.

The same release moves frps and frpc from v0.71.0 to v0.71.1 (a fixes-only frp release).

## Goals

- One optional field, `log.level`, on:
  - `Server.spec.log.level` → frps
  - `Client.spec.log.level` → frpc
  - `ServerPool.spec.clientTemplate.log.level` → frpc in every Client the pool generates
- Allowed values: exactly `trace`, `debug`, `info`, `warn`, `error`. The CRD rejects anything else;
  the default is `info`.
- Logs always go to stdout. The rendered `frps.toml` and `frpc.toml` say so explicitly:
  `log.to = "console"` and `log.level = "<level>"` (`"info"` when unset).
- A level change takes effect on its own, except on a Server with `restartPolicy: Manual`, where it
  waits for the owner like every other change (G1).
- frps and frpc run v0.71.1 everywhere the operator or its tests pin frp (G8).

## Non-goals

- Logging to a file (`log.to = <path>`), `log.maxDays`, `log.disablePrintColor`. The frps container
  has a read-only root filesystem, and on Kubernetes stdout is what `kubectl logs` and log
  collectors read.
- A different level per server inside a ServerPool (`servers[].log`). One level per pool.
- A staggered rollout when a ServerPool's level changes (G3); that belongs with the frpc replicas work.
- Any change to the operator's own logging.

## Anti-goals

- A level that is accepted by the API but makes frps or frpc crash or reject its config.
- Restarting a Client that stays at the default level **because of the log feature**. (This release
  restarts every frpc and frps once anyway, because of the frp image bump — G7.)

## Decisions

### Brainstorming

| # | Decision |
|---|---|
| D1 | One shared API type `LogSpec { level }` in `api/v1alpha1/types.go`, used by Client, Server and ServerPool's `clientTemplate` — the same pattern as `clientTemplate.podTemplate` reusing `ClientSpec_PodTemplate` |
| D2 | `level` is an enum `trace;debug;info;warn;error` with `+kubebuilder:default=info` |
| D3 | `log.to = "console"` and `log.level` are always rendered, for frps and frpc, with `info` when unset |
| D4 | **Server**: a level change restarts frps through the existing config hash |
| D5 | **Client**: frpc's hot reload does not apply log settings, so a level change recreates the frpc pod, through a pod annotation compared on reconcile, like the TLS-settings roll. A Client at the default level carries no annotation |
| D6 | **ServerPool**: `clientTemplate.log` is copied into every generated Client, like `clientTemplate.podTemplate`. Generated Clients are rebuilt from the pool on every reconcile, so editing a generated Client directly does not stick |
| D7 | Release as **v0.14.0** (chart 1.12.0): new CRD fields are a minor release |
| D8 | *Builder's call, found by the builder check:* the Client controller recreates the frpc pod for a TLS-settings change **before** it writes the new ConfigMap, so the new pod starts on the old `frpc.toml`, and the following hot reload does not apply TLS keys such as `serverName`. The log roll would inherit this. Fix it in the shared roll: write the ConfigMap first, then recreate the pod. This also fixes the shipped TLS roll |

### Grilling

| # | Decision |
|---|---|
| G1 | `restartPolicy: Manual` is honoured for log changes: the Server shows `ConfigSynced=False / RestartPending`, and the new level applies when the owner deletes the pod. After the upgrade, every Manual Server shows `RestartPending` (D3 changes every `frps.toml`) |
| G2 | Accepted: every `Automatic` Server restarts frps once after the upgrade, in exchange for an explicit `log.*` in `frps.toml` |
| G3 | Accepted: changing a ServerPool's `clientTemplate.log.level` recreates all its generated frpc pods at about the same time; documented next to the field |
| G4 | The TLS-roll ordering fix (D8) ships in the same release |
| G5 | The operator also checks the level (`ConfigSynced=False / InvalidConfig` and a Warning event), on top of the CRD enum, so an old CRD or a hand-edited object never reaches frp |
| G6 | v0.14.0 ships on its own, before the frpc replicas work (PR #13 takeover) |
| G7 | Accepted: the frp image bump restarts every Client's frpc and every Server's frps once after the upgrade |
| G8 | Bump to v0.71.1 together: `frpcImage` (`controllers/client_controller.go`), `frps.Image` (`pkg/frps/names.go`), the e2e cluster's frps (`test/e2e/manifests/infra.yaml`), and the docs that name the version (`AGENTS.md`). `make test-frpc-config` then verifies every rendered config against v0.71.1 |
| G9 | The v0.14.0 release notes list: the new field; the TLS-roll fix; the one-time restart of every frpc and frps; Manual Servers showing `RestartPending`; and frp v0.71.1's two behaviour changes — `healthCheck.maxFailed` now counts consecutive failures (the count resets after a success), and an HTTP `CONNECT` tunnel request (TCPMUX) with a body gets HTTP 400 |

## Verified facts this design relies on

- frp v0.71.0 `LogConfig.Complete()` defaults: `to = "console"` (stdout), `level = "info"`,
  `maxDays = 3` (`pkg/config/v1/common.go:119-123`).
- frpc's admin-API reload applies proxies and visitors, not common settings such as `log`.
- `frpc verify` / `frps verify` reject unknown keys.
- frp v0.71.1 (2026-10-14) is fixes-only; `fatedier/frpc:v0.71.1` and `fatedier/frps:v0.71.1` are
  published for amd64 and arm64 (among others).

## Testing

- Unit:
  - frps render: explicit console + level, default `info`
  - frpc render and model: default, explicit, invalid value
  - ServerPool copies `clientTemplate.log` into generated Clients
  - a Client pod is recreated on a level change, not at the default level, and the recreated pod
    starts on the **new** ConfigMap (D8; the same for a TLS change)
  - a Server restarts on a level change (Automatic) and goes `RestartPending` (Manual)
  - the operator rejects an unknown level (G5)
- Real binaries: `make test-frpc-config` against v0.71.1 covers configs with `log.level`, for frps
  and frpc.
- e2e: the existing suite runs against frps v0.71.1 (the e2e cluster's frps is bumped). No new e2e
  case: the keys are checked by the real binaries.

## Delivery

- CRDs regenerated (`charts/frp-operator/crds/crds.yaml`).
- README section and examples mention `log.level`; the ServerPool docs say a level change recreates
  all generated frpc pods at once (G3).
- v0.14.0 release notes per G9.

## Left to the builder

- Where exactly `log.*` sits in the rendered TOML.
- The annotation name for D5 and how the roll branch is shared with the TLS roll.
- Which examples get a `log.level` line.
{% endraw %}
