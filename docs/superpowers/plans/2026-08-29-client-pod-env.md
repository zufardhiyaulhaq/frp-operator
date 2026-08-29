# Client Pod Environment Variables Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional `podTemplate.env` list to the `Client` CRD that is copied onto the `frpc` container, so users can set `GOMEMLIMIT` (or anything else) without a mutating webhook.

**Architecture:** One new field on `ClientSpec_PodTemplate` (`[]corev1.EnvVar`), one assignment in `PodBuilder.Build()`, regenerated DeepCopy/CRDs, chart CRD sync + version bump, example and README updates. Additive and non-breaking.

**Tech Stack:** Go 1.23 · Kubebuilder v3 / controller-gen · controller-runtime · Helm

**Spec:** `docs/superpowers/specs/2026-08-29-client-pod-env-design.md`

---

## File Structure

**Modified:**
- `api/v1alpha1/client_types.go` — `Env` field on `ClientSpec_PodTemplate`
- `api/v1alpha1/zz_generated.deepcopy.go` — regenerated (do not hand-edit)
- `config/crd/bases/frp.zufardhiyaulhaq.com_clients.yaml` — regenerated
- `charts/frp-operator/crds/crds.yaml` — `Client` CRD section synced from generated file
- `charts/frp-operator/Chart.yaml` — version `1.5.0` → `1.6.0`
- `pkg/client/builder/pod_builder.go` — copy `Env` onto the container
- `pkg/client/builder/pod_builder_test.go` — two new tests
- `examples/operations/client-with-podtemplate.yaml` — `env` block with `GOMEMLIMIT`
- `README.md`, `charts/frp-operator/README.md`, `charts/frp-operator/README.md.gotmpl` — feature bullet

---

## Task 1: Failing tests for env passthrough

**Files:**
- Modify: `pkg/client/builder/pod_builder_test.go`

- [ ] **Step 1: Append tests**

```go
func TestPodBuilder_WithEnv(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Env: []corev1.EnvVar{
			{Name: "GOMEMLIMIT", Value: "80MiB"},
			{Name: "MEM_LIMIT", ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.memory"},
			}},
		},
	}
	pod, err := NewPodBuilder().SetName("test").SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").SetPodTemplate(pt).Build()
	if err != nil { t.Fatalf("Build() error = %v", err) }
	env := pod.Spec.Containers[0].Env
	if len(env) != 2 { t.Fatalf("Expected 2 env vars, got %d", len(env)) }
	if env[0].Name != "GOMEMLIMIT" || env[0].Value != "80MiB" { t.Errorf("bad env[0]: %+v", env[0]) }
	if env[1].ValueFrom == nil || env[1].ValueFrom.ResourceFieldRef == nil ||
		env[1].ValueFrom.ResourceFieldRef.Resource != "limits.memory" { t.Errorf("bad env[1]: %+v", env[1]) }
}

func TestPodBuilder_NoEnvByDefault(t *testing.T) {
	pod, err := NewPodBuilder().SetName("test").SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").SetPodTemplate(&frpv1alpha1.ClientSpec_PodTemplate{}).Build()
	if err != nil { t.Fatalf("Build() error = %v", err) }
	if len(pod.Spec.Containers[0].Env) != 0 { t.Errorf("Expected no env vars, got %v", pod.Spec.Containers[0].Env) }
}
```

- [ ] **Step 2: Run and confirm compile failure** (`Env` undefined on `ClientSpec_PodTemplate`)

```bash
go test ./pkg/client/builder/ -run 'TestPodBuilder_WithEnv|TestPodBuilder_NoEnvByDefault'
```

## Task 2: Add the CRD field

**Files:**
- Modify: `api/v1alpha1/client_types.go`

- [ ] **Step 1: Append to `ClientSpec_PodTemplate`** (after `SecurityContext`)

```go
	// +optional
	// Env is a list of environment variables to set in the FRP client container (e.g. GOMEMLIMIT)
	Env []corev1.EnvVar `json:"env,omitempty"`
```

- [ ] **Step 2: Regenerate**

```bash
make generate manifests
```

- [ ] **Step 3: Re-run tests — now they compile and `TestPodBuilder_WithEnv` fails** (0 env vars)

## Task 3: Copy env in the pod builder

**Files:**
- Modify: `pkg/client/builder/pod_builder.go`

- [ ] **Step 1: After the `Resources` block in `Build()`**

```go
	// Apply container env from PodTemplate
	if n.PodTemplate != nil && n.PodTemplate.Env != nil {
		container.Env = n.PodTemplate.Env
	}
```

- [ ] **Step 2: Run the full builder package and whole suite**

```bash
go test ./pkg/client/builder/
make test
make lint
```

## Task 4: Chart, example, docs

**Files:**
- Modify: `charts/frp-operator/crds/crds.yaml`, `charts/frp-operator/Chart.yaml`
- Modify: `examples/operations/client-with-podtemplate.yaml`
- Modify: `README.md`, `charts/frp-operator/README.md`, `charts/frp-operator/README.md.gotmpl`

- [ ] **Step 1: Sync the `Client` CRD in `charts/frp-operator/crds/crds.yaml`** with the regenerated `config/crd/bases/frp.zufardhiyaulhaq.com_clients.yaml` (the `env` schema block under `podTemplate`).

- [ ] **Step 2: Bump `charts/frp-operator/Chart.yaml` `version` to `1.6.0`.**

- [ ] **Step 3: Add to the example's `podTemplate`:**

```yaml
    env:
      - name: GOMEMLIMIT
        value: "200MiB"
```

- [ ] **Step 4: Update the feature bullet** in `README.md.gotmpl` to include "environment variables", then run `make readme` to regenerate both READMEs.

## Task 5: Manual verification

- [ ] Switch to the orbstack context, `make install run`, apply `examples/operations/client-with-podtemplate.yaml` (with its secrets), and confirm `kubectl get pod production-client-frpc -o yaml` shows the `GOMEMLIMIT` env and frpc logs are healthy.
- [ ] Do **not** commit — the user will review and commit.
