# AGENTS instructions

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

frp-operator is a Kubernetes operator that exposes private Kubernetes services to the Internet using FRP (Fast Reverse Proxy). Built with Kubebuilder v3 and controller-runtime.

## Build & Development Commands

```bash
# Build
make build              # Build operator binary → bin/manager
make docker-build       # Build container image

# Test
make test               # Run tests with coverage (output/coverage.html)
make test-frpc-config   # Render every feature/example/ServerPool config and validate it with the real frpc (needs Docker)
make test-e2e           # End-to-end on a private k3d cluster (frps in-cluster, real traffic); needs Docker, k3d, helm
make e2e-down           # Delete the e2e k3d cluster

# Lint & Format
make fmt                # Format code with go fmt
make vet                # Lint with go vet
make lint               # Static analysis with golangci-lint

# Code Generation (run after modifying api/v1alpha1 types)
make manifests          # Generate CRD YAML files
make generate           # Generate Go code (DeepCopy, etc.)
```

development manually run the operator, check the logs, apply the examples, and check if pods, configmap, and service is created properly, and FRP client logs is working as expected. 

## Releases

We are using Helm chart to release the FRP-Operator. after updating RBAC, CRDs, and charts version
```bash
make readme                # update readme in charts and root directory
make helm.create.releases. # create a new helm charts version
```

## Architecture

### Custom Resource Definitions (api/v1alpha1/)

- **Client**: FRP client instance connecting to an external FRP server
- **Upstream**: Service/port to expose through FRP (supports TCP, UDP, STCP, XTCP protocols)
- **Visitor**: Inbound tunnel to access another client's Upstreams (for P2P scenarios)
- **ServerPool**: pool of frps servers used to back `Service type=LoadBalancer` (loadBalancerClass `frp.zufardhiyaulhaq.com/frp`)

### Controllers (controllers/)

**ClientReconciler** is the primary active controller. Upstream and Visitor controllers are stubs - changes to those CRs are detected by ClientReconciler through list/watch.

**ServiceReconciler** binds claimed LoadBalancer Services to ServerPool servers and generates Client/Upstream CRs in the operator namespace (pure selection logic in `pkg/loadbalancer/`).

Reconciliation flow:
1. Fetch Client resource
2. List all Upstream & Visitor resources referencing this Client
3. Transform CRs → internal Config model (fetches secrets)
4. Build FRP TOML configuration
5. Create/Update ConfigMap, Service, Pod
6. Apply ConfigMap changes via FRP admin API reload
7. Requeue every 30 seconds

### Core Business Logic (pkg/client/)

- **builder/**: Builder pattern for K8s resources and FRP config
  - `configuration_builder.go`: Generates frpc TOML from Go templates
  - `configmap_builder.go`, `pod_builder.go`, `service_builder.go`: K8s resource builders
- **models/config.go**: Transforms CRs → internal Config, fetches secrets
- **handler/reload.go**: Calls FRP admin API to reload config without pod restart
- **handler/status.go**: Reads proxy status from the FRP admin API (`/api/status`) for metrics
- **utils/template.go**: FRP TOML configuration template

### Metrics (pkg/metrics/)

FRP-specific Prometheus gauges (`frp_client_*`, `frp_proxy_*`) registered on the controller-runtime registry. Written only by `ClientReconciler`; series are removed when a Client is deleted. Grafana dashboard JSON is hand-maintained at `dashboards/frp-operator.json`.

### Key Constants

- FRP image: `fatedier/frpc:v0.71.0`
- Admin port: 7400
- Default credentials: frpc-user / frpc-password (configurable via Secrets)

## Development Workflow

After modifying `api/v1alpha1/*_types.go`:
```bash
make manifests && make generate
```

To test locally with a cluster (always change to Kubernetes context to orbstack):
```bash
make install run
kubectl --context orbstack apply -f examples/simple/
```

The LoadBalancer controller (`ServiceReconciler`) needs to know which namespace holds `ServerPool`s and its generated `Client`/`Upstream` resources: `POD_NAMESPACE` env var, or `--operator-namespace` flag. `make run` sets `POD_NAMESPACE` to `default` if it isn't already set.

## Directory Structure

```
api/v1alpha1/       # CRD type definitions
controllers/        # Reconciliation logic
pkg/client/         # Core business logic (builders, models, handlers)
pkg/loadbalancer/   # LB allocator, naming, builders
config/             # Kustomize manifests (CRDs, RBAC, manager), mostly not being used. fix the RBAC & CRDs under charts instead
charts/             # Helm chart
dashboards/         # Grafana dashboard JSON
examples/           # Usage examples (simple, tcp-full, p2p, loadbalancer); every Client here is validated by test/frpcconfig
test/frpcconfig/    # frpc verify integration test (build tag frpcverify)
test/e2e/           # e2e suite (build tag e2e); untagged helpers are unit-tested by make test
```