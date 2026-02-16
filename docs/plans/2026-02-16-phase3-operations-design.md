# Phase 3: Operations & Observability Design

**Date:** 2026-02-16
**Compatibility:** Strict backwards compatibility (v1alpha1 additive changes only)

## Use Case

Better debugging, monitoring, and pod customization for production deployments.

## Current State

- **Status:** All CRDs have empty status fields
- **Events:** No Kubernetes events are emitted
- **Pod Customization:** No way to set resources, node selectors, or tolerations

## Proposed Changes

### 3A: Status Fields

Add meaningful status to all CRDs for observability.

#### ClientStatus

```go
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

**Condition Types:**
- `Ready` - Client pod is running and connected
- `ConfigSynced` - ConfigMap is up-to-date with desired state

#### UpstreamStatus

```go
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

#### VisitorStatus

```go
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

### 3B: Kubernetes Events

Emit events for key lifecycle moments to aid debugging.

```go
// Event reasons
const (
    ReasonClientConnected     = "ClientConnected"
    ReasonClientDisconnected  = "ClientDisconnected"
    ReasonProxyRegistered     = "ProxyRegistered"
    ReasonProxyFailed         = "ProxyFailed"
    ReasonConfigReloaded      = "ConfigReloaded"
    ReasonConfigReloadFailed  = "ConfigReloadFailed"
    ReasonVisitorConnected    = "VisitorConnected"
)

// In reconcile loop
r.Recorder.Event(client, corev1.EventTypeNormal, ReasonClientConnected,
    fmt.Sprintf("Connected to FRP server %s:%d", host, port))

r.Recorder.Event(client, corev1.EventTypeWarning, ReasonConfigReloadFailed,
    "Failed to reload config: admin API unreachable")

r.Recorder.Event(upstream, corev1.EventTypeNormal, ReasonProxyRegistered,
    fmt.Sprintf("TCP proxy registered on remote port %d", remotePort))
```

### 3C: Pod Customization

Allow users to customize the FRP client pod for production requirements.

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

### 3D: Admin Server Enhancements

Add pprof support for profiling.

```go
type ClientSpec_Server_AdminServer struct {
    Port     int                                     `json:"port"`
    Username *ClientSpec_Server_AdminServer_Username `json:"username"`
    Password *ClientSpec_Server_AdminServer_Password `json:"password"`
    // +optional
    // +kubebuilder:default=false
    // PprofEnable enables Go profiling handlers on the admin server
    PprofEnable bool `json:"pprofEnable,omitempty"`
}
```

## Example Usage

### Client with Full Pod Customization

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
      node-type: network
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "frp"
        effect: "NoSchedule"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: frp-client
              topologyKey: kubernetes.io/hostname
    labels:
      app.kubernetes.io/part-of: my-application
      environment: production
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "7400"
    serviceAccountName: frp-client-sa
    priorityClassName: high-priority
```

### Expected Status Output

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: my-client
spec: ...
status:
  phase: Running
  message: "Connected to frp.example.com:7000"
  lastReconnect: "2026-02-16T10:30:00Z"
  upstreamCount: 3
  visitorCount: 1
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-02-16T10:30:00Z"
      reason: Connected
      message: "FRP client connected successfully"
    - type: ConfigSynced
      status: "True"
      lastTransitionTime: "2026-02-16T10:31:00Z"
      reason: ConfigMapUpdated
      message: "Configuration synchronized"
```

### Expected Events

```
$ kubectl describe client my-client
...
Events:
  Type    Reason              Age   Message
  ----    ------              ----  -------
  Normal  ClientConnected     5m    Connected to FRP server frp.example.com:7000
  Normal  ConfigReloaded      3m    Configuration reloaded successfully
  Normal  ProxyRegistered     3m    TCP proxy "my-upstream" registered on port 8080
```

## Files to Modify

### API Types
- `api/v1alpha1/client_types.go` - ClientStatus, PodTemplate
- `api/v1alpha1/upstream_types.go` - UpstreamStatus
- `api/v1alpha1/visitor_types.go` - VisitorStatus

### Controllers
- `controllers/client_controller.go` - Status updates, event recording
- Add event recorder to controller struct

### Business Logic
- `pkg/client/builder/pod_builder.go` - Apply pod template customizations
- `pkg/client/utils/template.go` - Add pprofEnable to template

### RBAC
- Update ClusterRole to allow event creation

### Tests
- Unit tests for pod builder with all customization options
- Controller tests verifying status updates
- Controller tests verifying event emission

## TOML Template Addition

```toml
webServer.addr = "{{ .Common.AdminAddress }}"
webServer.port = {{ .Common.AdminPort }}
webServer.user = "{{ .Common.AdminUsername }}"
webServer.password = "{{ .Common.AdminPassword }}"
{{ if .Common.PprofEnable }}
webServer.pprofEnable = true
{{ end }}
```

## Implementation Notes

### Status Update Timing

Status should be updated:
1. When pod is created/updated (phase: Pending -> Running)
2. When config reload succeeds/fails
3. When connection to server is established/lost (requires FRP API polling)
4. Periodically during reconciliation (update counts)

### Event Recording Setup

```go
// In controller setup
func (r *ClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
    r.Recorder = mgr.GetEventRecorderFor("client-controller")
    return ctrl.NewControllerManagedBy(mgr).
        For(&frpv1alpha1.Client{}).
        // ...
        Complete(r)
}
```

### Pod Builder Changes

```go
func (b *PodBuilder) Build() *corev1.Pod {
    pod := &corev1.Pod{
        // ... existing fields ...
    }

    if b.config.PodTemplate != nil {
        pt := b.config.PodTemplate
        pod.Spec.Containers[0].Resources = pt.Resources
        pod.Spec.NodeSelector = pt.NodeSelector
        pod.Spec.Tolerations = pt.Tolerations
        pod.Spec.Affinity = pt.Affinity
        pod.Spec.ServiceAccountName = pt.ServiceAccountName
        pod.Spec.ImagePullSecrets = pt.ImagePullSecrets
        pod.Spec.PriorityClassName = pt.PriorityClassName
        pod.Spec.SecurityContext = pt.SecurityContext

        // Merge labels and annotations
        for k, v := range pt.Labels {
            pod.Labels[k] = v
        }
        for k, v := range pt.Annotations {
            pod.Annotations[k] = v
        }
    }

    return pod
}
```

## Validation Rules

1. Resource requests must not exceed limits
2. PriorityClassName must exist in the cluster (runtime validation)
3. ServiceAccountName must exist in the namespace (runtime validation)

## Testing Strategy

1. Unit tests for pod builder with all template options
2. Controller tests for status condition updates
3. Controller tests for event emission
4. Integration tests verifying pod has expected configuration
