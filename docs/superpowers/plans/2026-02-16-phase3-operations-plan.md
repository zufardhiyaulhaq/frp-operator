# Phase 3: Operations & Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add status fields, Kubernetes events, and pod template customization for production deployments.

**Architecture:** Extend all CRD Status structs with phase/conditions, add event recorder to controller, extend ClientSpec with PodTemplate for resource limits/scheduling.

**Tech Stack:** Go, Kubebuilder v3, controller-runtime, Kubernetes Events API

---

## Task 1: Add Status Fields to Client CRD

**Files:**
- Modify: `api/v1alpha1/client_types.go:62-65`

**Step 1: Update ClientStatus struct**

Replace the empty `ClientStatus` in `api/v1alpha1/client_types.go`:

```go
// ClientStatus defines the observed state of Client
type ClientStatus struct {
	// +optional
	// Phase indicates the current state: Pending, Running, Failed, Unknown
	Phase string `json:"phase,omitempty"`
	// +optional
	// Message provides human-readable status information
	Message string `json:"message,omitempty"`
	// +optional
	// LastReconnect is the timestamp of the last successful reconnection
	LastReconnect *metav1.Time `json:"lastReconnect,omitempty"`
	// +optional
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	// UpstreamCount is the number of upstreams associated with this client
	UpstreamCount int `json:"upstreamCount,omitempty"`
	// +optional
	// VisitorCount is the number of visitors associated with this client
	VisitorCount int `json:"visitorCount,omitempty"`
}
```

**Step 2: Add kubebuilder markers for status**

Update the Client struct markers:

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Upstreams",type=integer,JSONPath=`.status.upstreamCount`
//+kubebuilder:printcolumn:name="Visitors",type=integer,JSONPath=`.status.visitorCount`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Client is the Schema for the clients API
type Client struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClientSpec   `json:"spec,omitempty"`
	Status ClientStatus `json:"status,omitempty"`
}
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/client_types.go config/crd/
git commit -m "feat(api): add status fields to Client CRD"
```

---

## Task 2: Add Status Fields to Upstream and Visitor CRDs

**Files:**
- Modify: `api/v1alpha1/upstream_types.go:122-124`
- Modify: `api/v1alpha1/visitor_types.go:67-69`

**Step 1: Update UpstreamStatus**

```go
// UpstreamStatus defines the observed state of Upstream
type UpstreamStatus struct {
	// +optional
	// Phase indicates the current state: Pending, Active, Failed
	Phase string `json:"phase,omitempty"`
	// +optional
	// Message provides human-readable status information
	Message string `json:"message,omitempty"`
	// +optional
	// RegisteredAt is when the proxy was registered with the server
	RegisteredAt *metav1.Time `json:"registeredAt,omitempty"`
}
```

**Step 2: Add printcolumn markers to Upstream**

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Client",type=string,JSONPath=`.spec.client`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
```

**Step 3: Update VisitorStatus**

```go
// VisitorStatus defines the observed state of Visitor
type VisitorStatus struct {
	// +optional
	// Phase indicates the current state: Pending, Active, Failed
	Phase string `json:"phase,omitempty"`
	// +optional
	// Message provides human-readable status information
	Message string `json:"message,omitempty"`
	// +optional
	// ConnectedAt is when the visitor established connection
	ConnectedAt *metav1.Time `json:"connectedAt,omitempty"`
}
```

**Step 4: Add printcolumn markers to Visitor**

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Client",type=string,JSONPath=`.spec.client`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
```

**Step 5: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 6: Commit**

```bash
git add api/v1alpha1/upstream_types.go api/v1alpha1/visitor_types.go config/crd/
git commit -m "feat(api): add status fields to Upstream and Visitor CRDs"
```

---

## Task 3: Add Status Constants and Helpers

**Files:**
- Create: `pkg/client/status/status.go`

**Step 1: Create status package**

Run: `mkdir -p pkg/client/status`

**Step 2: Create status.go**

```go
package status

const (
	// Client phases
	ClientPhasePending = "Pending"
	ClientPhaseRunning = "Running"
	ClientPhaseFailed  = "Failed"
	ClientPhaseUnknown = "Unknown"

	// Upstream phases
	UpstreamPhasePending = "Pending"
	UpstreamPhaseActive  = "Active"
	UpstreamPhaseFailed  = "Failed"

	// Visitor phases
	VisitorPhasePending = "Pending"
	VisitorPhaseActive  = "Active"
	VisitorPhaseFailed  = "Failed"

	// Condition types
	ConditionTypeReady      = "Ready"
	ConditionTypeConfigSync = "ConfigSynced"

	// Condition reasons
	ReasonPodCreated       = "PodCreated"
	ReasonPodRunning       = "PodRunning"
	ReasonPodFailed        = "PodFailed"
	ReasonConfigMapUpdated = "ConfigMapUpdated"
	ReasonConfigReloaded   = "ConfigReloaded"
	ReasonConfigReloadFailed = "ConfigReloadFailed"
)
```

**Step 3: Commit**

```bash
git add pkg/client/status/
git commit -m "feat: add status constants and helpers"
```

---

## Task 4: Add Event Recorder to Controller

**Files:**
- Modify: `controllers/client_controller.go`

**Step 1: Add event recorder to ClientReconciler struct**

```go
import (
	"k8s.io/client-go/tools/record"
)

type ClientReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}
```

**Step 2: Add event constants**

```go
const (
	EventReasonClientConnected    = "ClientConnected"
	EventReasonClientDisconnected = "ClientDisconnected"
	EventReasonProxyRegistered    = "ProxyRegistered"
	EventReasonProxyFailed        = "ProxyFailed"
	EventReasonConfigReloaded     = "ConfigReloaded"
	EventReasonConfigReloadFailed = "ConfigReloadFailed"
)
```

**Step 3: Update SetupWithManager to add event recorder**

```go
func (r *ClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("client-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&frpv1alpha1.Client{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
```

**Step 4: Add events in reconcile loop**

Add after successful pod creation:

```go
r.Recorder.Event(clientObject, corev1.EventTypeNormal, EventReasonClientConnected,
	fmt.Sprintf("FRP client pod created for server %s:%d",
		clientObject.Spec.Server.Host, clientObject.Spec.Server.Port))
```

Add after config reload:

```go
r.Recorder.Event(clientObject, corev1.EventTypeNormal, EventReasonConfigReloaded,
	"Configuration reloaded successfully")
```

Add on config reload failure:

```go
r.Recorder.Event(clientObject, corev1.EventTypeWarning, EventReasonConfigReloadFailed,
	fmt.Sprintf("Failed to reload config: %v", err))
```

**Step 5: Update RBAC markers**

Add to controller file:

```go
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

**Step 6: Run code generation**

Run: `make manifests`
Expected: RBAC updated

**Step 7: Commit**

```bash
git add controllers/client_controller.go config/rbac/
git commit -m "feat: add event recorder to ClientReconciler"
```

---

## Task 5: Update Status in Reconcile Loop

**Files:**
- Modify: `controllers/client_controller.go`

**Step 1: Add status update helper function**

```go
func (r *ClientReconciler) updateClientStatus(ctx context.Context, client *frpv1alpha1.Client,
	phase, message string, upstreamCount, visitorCount int) error {

	client.Status.Phase = phase
	client.Status.Message = message
	client.Status.UpstreamCount = upstreamCount
	client.Status.VisitorCount = visitorCount

	return r.Status().Update(ctx, client)
}

func (r *ClientReconciler) setCondition(client *frpv1alpha1.Client,
	conditionType string, status metav1.ConditionStatus, reason, message string) {

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update or append
	for i, c := range client.Status.Conditions {
		if c.Type == conditionType {
			client.Status.Conditions[i] = condition
			return
		}
	}
	client.Status.Conditions = append(client.Status.Conditions, condition)
}
```

**Step 2: Update reconcile to set status**

After listing upstreams and visitors:

```go
upstreamCount := len(upstreamList.Items)
visitorCount := len(visitorList.Items)
```

After pod creation/update:

```go
r.setCondition(clientObject, status.ConditionTypeReady,
	metav1.ConditionTrue, status.ReasonPodRunning, "FRP client pod is running")
r.updateClientStatus(ctx, clientObject, status.ClientPhaseRunning,
	fmt.Sprintf("Connected to %s:%d", clientObject.Spec.Server.Host, clientObject.Spec.Server.Port),
	upstreamCount, visitorCount)
```

On error:

```go
r.setCondition(clientObject, status.ConditionTypeReady,
	metav1.ConditionFalse, status.ReasonPodFailed, err.Error())
r.updateClientStatus(ctx, clientObject, status.ClientPhaseFailed, err.Error(), 0, 0)
```

**Step 3: Commit**

```bash
git add controllers/client_controller.go
git commit -m "feat: update Client status in reconcile loop"
```

---

## Task 6: Add PodTemplate to ClientSpec

**Files:**
- Modify: `api/v1alpha1/client_types.go`

**Step 1: Add PodTemplate type**

```go
type ClientSpec struct {
	Server ClientSpec_Server `json:"server"`
	// +optional
	// PodTemplate allows customization of the FRP client pod
	PodTemplate *ClientSpec_PodTemplate `json:"podTemplate,omitempty"`
}

type ClientSpec_PodTemplate struct {
	// +optional
	// Resources specifies compute resources for the container
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	// NodeSelector constrains pod scheduling to nodes with matching labels
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	// Tolerations allow the pod to schedule onto nodes with matching taints
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	// Affinity specifies scheduling constraints
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// +optional
	// Labels are additional labels to add to the pod
	Labels map[string]string `json:"labels,omitempty"`
	// +optional
	// Annotations are additional annotations to add to the pod
	Annotations map[string]string `json:"annotations,omitempty"`
	// +optional
	// ServiceAccountName is the name of the ServiceAccount to use
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +optional
	// ImagePullSecrets are references to secrets for pulling images
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// +optional
	// PriorityClassName is the name of the PriorityClass
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// +optional
	// SecurityContext holds pod-level security attributes
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
}
```

**Step 2: Add import for corev1**

```go
import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

**Step 3: Run code generation**

Run: `make generate && make manifests`
Expected: No errors

**Step 4: Commit**

```bash
git add api/v1alpha1/client_types.go config/crd/
git commit -m "feat(api): add PodTemplate to ClientSpec"
```

---

## Task 7: Update PodBuilder to Apply PodTemplate

**Files:**
- Modify: `pkg/client/builder/pod_builder.go`
- Create: `pkg/client/builder/pod_builder_test.go`

**Step 1: Add PodTemplate field to PodBuilder**

```go
import (
	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodBuilder struct {
	Name        string
	Namespace   string
	Image       string
	PodTemplate *frpv1alpha1.ClientSpec_PodTemplate
}

func (n *PodBuilder) SetPodTemplate(pt *frpv1alpha1.ClientSpec_PodTemplate) *PodBuilder {
	n.PodTemplate = pt
	return n
}
```

**Step 2: Update Build method to apply PodTemplate**

```go
func (n *PodBuilder) Build() (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name + "-frpc",
			Namespace: n.Namespace,
			Labels:    n.BuildLabels(),
			Annotations: map[string]string{
				"sidecar.istio.io/inject":                "false",
				"linkerd.io/inject":                      "disabled",
				"kuma.io/sidecar-injection":              "disabled",
				"appmesh.k8s.aws/sidecarInjectorWebhook": "disabled",
				"injector.nsm.nginx.com/auto-inject":     "false",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "frpc",
					Image:   n.Image,
					Command: []string{"frpc", "-c", "/frp/config.toml"},
					Ports: []corev1.ContainerPort{
						{ContainerPort: int32(4040)},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      n.Name + "-frpc-config",
							MountPath: "/frp",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: n.Name + "-frpc-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: n.Name + "-frpc-config",
							},
						},
					},
				},
			},
		},
	}

	// Apply PodTemplate customizations
	if n.PodTemplate != nil {
		n.applyPodTemplate(pod)
	}

	return pod, nil
}

func (n *PodBuilder) applyPodTemplate(pod *corev1.Pod) {
	pt := n.PodTemplate

	// Apply container resources
	if pt.Resources.Limits != nil || pt.Resources.Requests != nil {
		pod.Spec.Containers[0].Resources = pt.Resources
	}

	// Apply scheduling
	if len(pt.NodeSelector) > 0 {
		pod.Spec.NodeSelector = pt.NodeSelector
	}
	if len(pt.Tolerations) > 0 {
		pod.Spec.Tolerations = pt.Tolerations
	}
	if pt.Affinity != nil {
		pod.Spec.Affinity = pt.Affinity
	}

	// Apply service account
	if pt.ServiceAccountName != "" {
		pod.Spec.ServiceAccountName = pt.ServiceAccountName
	}

	// Apply image pull secrets
	if len(pt.ImagePullSecrets) > 0 {
		pod.Spec.ImagePullSecrets = pt.ImagePullSecrets
	}

	// Apply priority
	if pt.PriorityClassName != "" {
		pod.Spec.PriorityClassName = pt.PriorityClassName
	}

	// Apply security context
	if pt.SecurityContext != nil {
		pod.Spec.SecurityContext = pt.SecurityContext
	}

	// Merge labels
	for k, v := range pt.Labels {
		pod.Labels[k] = v
	}

	// Merge annotations
	for k, v := range pt.Annotations {
		pod.Annotations[k] = v
	}
}
```

**Step 3: Write test for PodTemplate**

Create `pkg/client/builder/pod_builder_test.go`:

```go
package builder

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodBuilder_WithPodTemplate(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		NodeSelector: map[string]string{
			"kubernetes.io/os": "linux",
		},
		Labels: map[string]string{
			"custom-label": "value",
		},
		Annotations: map[string]string{
			"custom-annotation": "value",
		},
		ServiceAccountName: "frp-sa",
		PriorityClassName:  "high-priority",
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check resources
	if pod.Spec.Containers[0].Resources.Requests.Cpu().String() != "100m" {
		t.Errorf("Expected CPU request 100m, got %s", pod.Spec.Containers[0].Resources.Requests.Cpu().String())
	}

	// Check node selector
	if pod.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Errorf("Expected node selector kubernetes.io/os=linux")
	}

	// Check labels
	if pod.Labels["custom-label"] != "value" {
		t.Errorf("Expected custom-label=value")
	}

	// Check service account
	if pod.Spec.ServiceAccountName != "frp-sa" {
		t.Errorf("Expected ServiceAccountName frp-sa, got %s", pod.Spec.ServiceAccountName)
	}

	// Check priority class
	if pod.Spec.PriorityClassName != "high-priority" {
		t.Errorf("Expected PriorityClassName high-priority, got %s", pod.Spec.PriorityClassName)
	}
}
```

**Step 4: Run test**

Run: `go test ./pkg/client/builder/... -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/client/builder/pod_builder.go pkg/client/builder/pod_builder_test.go
git commit -m "feat: apply PodTemplate customizations in PodBuilder"
```

---

## Task 8: Update Controller to Pass PodTemplate

**Files:**
- Modify: `controllers/client_controller.go`

**Step 1: Update pod builder call**

Find where PodBuilder is used and add:

```go
podBuilder := builder.NewPodBuilder().
	SetName(clientObject.Name).
	SetNamespace(clientObject.Namespace).
	SetImage("fatedier/frpc:v0.65.0")

if clientObject.Spec.PodTemplate != nil {
	podBuilder.SetPodTemplate(clientObject.Spec.PodTemplate)
}

pod, err := podBuilder.Build()
```

**Step 2: Commit**

```bash
git add controllers/client_controller.go
git commit -m "feat: pass PodTemplate to PodBuilder in controller"
```

---

## Task 9: Add pprofEnable to Admin Server

**Files:**
- Modify: `api/v1alpha1/client_types.go`
- Modify: `pkg/client/models/config.go`
- Modify: `pkg/client/utils/template.go`

**Step 1: Add PprofEnable field**

```go
type ClientSpec_Server_AdminServer struct {
	Port     int                                     `json:"port"`
	Username *ClientSpec_Server_AdminServer_Username `json:"username"`
	Password *ClientSpec_Server_AdminServer_Password `json:"password"`
	// +optional
	// +kubebuilder:default=false
	PprofEnable bool `json:"pprofEnable,omitempty"`
}
```

**Step 2: Add to Common model**

```go
type Common struct {
	// ... existing fields ...
	PprofEnable bool
}
```

**Step 3: Add to template**

```go
{{ if .Common.PprofEnable }}
webServer.pprofEnable = true
{{ end }}
```

**Step 4: Update NewConfig**

```go
if clientObject.Spec.Server.AdminServer != nil {
	// ... existing code ...
	config.Common.PprofEnable = clientObject.Spec.Server.AdminServer.PprofEnable
}
```

**Step 5: Run tests**

Run: `make test`
Expected: All PASS

**Step 6: Commit**

```bash
git add api/v1alpha1/client_types.go pkg/client/models/config.go pkg/client/utils/template.go
git commit -m "feat: add pprofEnable to admin server"
```

---

## Task 10: Add Example and Update Helm Chart

**Files:**
- Create: `examples/operations/`
- Modify: `charts/frp-operator/`

**Step 1: Create operations example**

Create `examples/operations/client-with-podtemplate.yaml`:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: production-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    adminServer:
      port: 7400
      pprofEnable: true
      username:
        secret:
          name: admin-creds
          key: username
      password:
        secret:
          name: admin-creds
          key: password
  podTemplate:
    resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
    nodeSelector:
      kubernetes.io/os: linux
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "frp"
        effect: "NoSchedule"
    labels:
      app.kubernetes.io/part-of: my-application
    annotations:
      prometheus.io/scrape: "true"
    serviceAccountName: frp-client-sa
    priorityClassName: high-priority
```

**Step 2: Update Helm CRDs**

Run: `cp config/crd/bases/*.yaml charts/frp-operator/crds/`

**Step 3: Update RBAC in Helm chart**

Ensure `charts/frp-operator/templates/rbac.yaml` includes events permission:

```yaml
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Step 4: Run all tests**

Run: `make test`
Expected: All PASS

**Step 5: Commit**

```bash
git add examples/operations/ charts/frp-operator/
git commit -m "feat: complete Phase 3 - operations and observability"
```

---

## Summary

Phase 3 adds operations features:
- **Status Fields**: Phase, message, conditions on all CRDs
- **Kubernetes Events**: ClientConnected, ConfigReloaded, etc.
- **Pod Template**: Resources, scheduling, labels, annotations
- **pprof Support**: Enable profiling on admin server

Files modified:
- `api/v1alpha1/client_types.go` - Status, PodTemplate, pprofEnable
- `api/v1alpha1/upstream_types.go` - Status fields
- `api/v1alpha1/visitor_types.go` - Status fields
- `pkg/client/status/status.go` - Status constants
- `pkg/client/builder/pod_builder.go` - PodTemplate support
- `controllers/client_controller.go` - Events, status updates
- `examples/operations/` - Example manifests
