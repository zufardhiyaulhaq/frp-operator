// api/v1alpha1/serverpool_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PortRange is a single port ("443") or an inclusive range ("8000-9000").
// +kubebuilder:validation:Pattern=`^\d+(-\d+)?$`
type PortRange string

const (
	AllocationPolicyAuto     = "Auto"
	AllocationPolicyExplicit = "Explicit"
	ClientModePerService     = "PerService"
	ClientModeShared         = "Shared"
)

// ServerPoolSpec defines a set of frps servers that LoadBalancer Services can be bound to.
type ServerPoolSpec struct {
	// +kubebuilder:validation:Enum=Auto;Explicit
	// +kubebuilder:default=Auto
	// +optional
	// AllocationPolicy: Auto pools are eligible for Services without a server-pool annotation;
	// Explicit pools are used only when a Service names them.
	AllocationPolicy string `json:"allocationPolicy,omitempty"`
	// +kubebuilder:validation:Enum=PerService;Shared
	// +kubebuilder:default=PerService
	// +optional
	// ClientMode: PerService creates one frpc Client per Service per server; Shared creates one
	// Client per server shared by every Service bound to it.
	ClientMode string `json:"clientMode,omitempty"`
	// +optional
	// AllowedPorts restricts which Service ports may be claimed on servers of this pool.
	// Empty means any port. A server's own allowedPorts replaces this list.
	AllowedPorts []PortRange `json:"allowedPorts,omitempty"`
	// +kubebuilder:validation:MinItems=1
	Servers []ServerPoolServer `json:"servers"`
	// +optional
	// ClientTemplate is applied to every Client generated from this pool.
	ClientTemplate *ServerPoolClientTemplate `json:"clientTemplate,omitempty"`
}

// ServerPoolServer is one frps server. Fields mirror Client.spec.server.
type ServerPoolServer struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// Name is the stable key used in annotations and generated resource names.
	Name string `json:"name"`
	// Host is the address frpc dials.
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// PublicAddress (IP or hostname) is written to Service status.loadBalancer.ingress.
	PublicAddress string `json:"publicAddress"`
	// +optional
	// AllowedPorts replaces the pool-level allowedPorts for this server.
	AllowedPorts []PortRange `json:"allowedPorts,omitempty"`
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	// TransportProtocol is the frpc→frps transport (Client.spec.server.protocol). wss needs a
	// TLS terminator in front of frps, because frps itself only accepts ws.
	TransportProtocol *string                          `json:"transportProtocol,omitempty"`
	Authentication    ClientSpec_Server_Authentication `json:"authentication"`
	// +optional
	AdminServer *ClientSpec_Server_AdminServer `json:"adminServer,omitempty"`
	// +optional
	TLS *ClientSpec_Server_TLS `json:"tls,omitempty"`
	// +optional
	Transport *ClientSpec_Server_Transport `json:"transport,omitempty"`
}

type ServerPoolClientTemplate struct {
	// +optional
	PodTemplate *ClientSpec_PodTemplate `json:"podTemplate,omitempty"`
}

type AllocatedPort struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
	// Service is "<namespace>/<name>" of the bound Service.
	Service string `json:"service"`
}

type ServerPoolServerStatus struct {
	Name string `json:"name"`
	// +optional
	AllocatedPorts []AllocatedPort `json:"allocatedPorts,omitempty"`
}

// ServerPoolStatus is a projection of current allocations; generated Upstreams are the source of truth.
type ServerPoolStatus struct {
	// +optional
	Servers []ServerPoolServerStatus `json:"servers,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.spec.allocationPolicy`
//+kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.clientMode`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ServerPool is the Schema for the serverpools API
type ServerPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerPoolSpec   `json:"spec,omitempty"`
	Status ServerPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerPoolList contains a list of ServerPool
type ServerPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerPool{}, &ServerPoolList{})
}
