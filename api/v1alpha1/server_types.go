package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RestartPolicyAutomatic = "Automatic"
	RestartPolicyManual    = "Manual"
)

// SecretName names a Secret in the Server's namespace; the field using it fixes the keys.
type SecretName struct {
	Name string `json:"name"`
}

// SecretNameRef references a whole Secret: {secret: {name: ...}}.
type SecretNameRef struct {
	Secret SecretName `json:"secret"`
}

// ServerSpec is an frps server run by the operator: one frps pod behind a public Service.
type ServerSpec struct {
	// Bind is where frpc clients connect (the tunnel).
	Bind ServerSpec_Bind `json:"bind"`
	// +optional
	// AllowPorts are the remote ports TCP and UDP Upstreams may claim. Each port is exposed on the
	// public Service with its protocol; frps itself gets the union of both lists. Unset: TCP and
	// UDP Upstreams are refused.
	AllowPorts *ServerSpec_AllowPorts `json:"allowPorts,omitempty"`
	// +optional
	// Vhost are the shared HTTP, HTTPS and TCPMUX ports, routed by domain.
	Vhost *ServerSpec_Vhost `json:"vhost,omitempty"`
	// +optional
	// Authentication of frpc clients. Without a token Secret the operator generates a token and
	// publishes it in the <server>-connection Secret.
	Authentication *ServerSpec_Authentication `json:"authentication,omitempty"`
	// +kubebuilder:validation:Enum=Automatic;Manual
	// +kubebuilder:default=Automatic
	// +optional
	// RestartPolicy: frps cannot reload its configuration. Automatic restarts the frps pod on any
	// change; Manual waits until you delete the pod (ConfigSynced=False, reason RestartPending).
	RestartPolicy string `json:"restartPolicy,omitempty"`
	// +optional
	// Service is the public Service in front of frps.
	Service *ServerSpec_Service `json:"service,omitempty"`
	// +optional
	// Dashboard is frps's web UI and admin API on port 7500, exposed only through its own Service.
	Dashboard *ServerSpec_Dashboard `json:"dashboard,omitempty"`
	// +optional
	Metrics *ServerSpec_Metrics `json:"metrics,omitempty"`
	// +optional
	// PodTemplate customizes the frps pod. Its securityContext replaces the operator's pod-level
	// one, including the sysctl that lets the non-root frps bind ports below 1024.
	PodTemplate *ClientSpec_PodTemplate `json:"podTemplate,omitempty"`
	// +optional
	// Log configures frps's logging (always to stdout). Changing it restarts frps.
	Log *LogSpec `json:"log,omitempty"`
}

type ServerSpec_Bind struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// Port accepts tcp and websocket clients (frps bindPort).
	Port int32 `json:"port"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	// KCPPort accepts kcp clients over UDP (frps kcpBindPort). It may equal port.
	KCPPort *int32 `json:"kcpPort,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	// QUICPort accepts quic clients over UDP (frps quicBindPort).
	QUICPort *int32 `json:"quicPort,omitempty"`
	// +optional
	// TLS secures frpc ↔ frps.
	TLS *ServerSpec_Bind_TLS `json:"tls,omitempty"`
}

type ServerSpec_Bind_TLS struct {
	// +optional
	// Certificate is a Secret with tls.crt and tls.key, and optionally ca.crt, which is published in
	// the connection Secret. Unset: frps uses a random self-signed certificate clients cannot verify.
	Certificate *SecretNameRef `json:"certificate,omitempty"`
	// +optional
	// ClientCA is a Secret with ca.crt. When set, every frpc must present a client certificate
	// signed by it (mTLS).
	ClientCA *SecretNameRef `json:"clientCA,omitempty"`
	// +kubebuilder:default=true
	// +optional
	// Force rejects frpc connections without TLS. Turn it off only for wss behind an external TLS
	// terminator.
	Force *bool `json:"force,omitempty"`
}

type ServerSpec_AllowPorts struct {
	// +optional
	TCP []PortRange `json:"tcp,omitempty"`
	// +optional
	UDP []PortRange `json:"udp,omitempty"`
}

type ServerSpec_Vhost struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	HTTPPort *int32 `json:"httpPort,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	// HTTPSPort routes by TLS SNI and passes the stream through; TLS ends at the backend.
	HTTPSPort *int32 `json:"httpsPort,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	TCPMuxPort *int32 `json:"tcpmuxPort,omitempty"`
	// +optional
	// SubdomainHost lets HTTP, HTTPS and TCPMUX Upstreams use "<subdomain>.<subdomainHost>". frps then
	// rejects customDomains under it: those names can only be claimed with subdomain.
	SubdomainHost string `json:"subdomainHost,omitempty"`
}

type ServerSpec_Authentication struct {
	// +optional
	Token *ClientSpec_Server_Authentication_Token `json:"token,omitempty"`
}

type ServerSpec_Service struct {
	// +kubebuilder:validation:Enum=LoadBalancer;ClusterIP
	// +kubebuilder:default=LoadBalancer
	// +optional
	// Type of the public Service. NodePort is not supported: it remaps ports, and frps opens each
	// remote port itself.
	Type string `json:"type,omitempty"`
	// +optional
	// Annotations, for example a cloud provider's static IP annotation.
	Annotations map[string]string `json:"annotations,omitempty"`
	// +optional
	LoadBalancerClass string `json:"loadBalancerClass,omitempty"`
	// +kubebuilder:validation:Enum=Cluster;Local
	// +optional
	ExternalTrafficPolicy string `json:"externalTrafficPolicy,omitempty"`
}

type ServerSpec_Dashboard struct {
	// +optional
	// Enabled starts the frps dashboard and admin API on port 7500.
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	// Credentials is a Secret with username and password. Unset: generated into <server>-dashboard.
	Credentials *SecretNameRef `json:"credentials,omitempty"`
	// +optional
	// TLS serves the dashboard, admin API and /metrics over HTTPS.
	TLS *ServerSpec_Dashboard_TLS `json:"tls,omitempty"`
	// +optional
	// Service exposes the dashboard, admin API and /metrics. There is never an Ingress.
	Service *ServerSpec_Dashboard_Service `json:"service,omitempty"`
}

type ServerSpec_Dashboard_TLS struct {
	// Certificate is a Secret with tls.crt and tls.key.
	Certificate SecretNameRef `json:"certificate"`
}

type ServerSpec_Dashboard_Service struct {
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default=ClusterIP
	// +optional
	Type string `json:"type,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +optional
	LoadBalancerClass string `json:"loadBalancerClass,omitempty"`
	// +optional
	LoadBalancerSourceRanges []string `json:"loadBalancerSourceRanges,omitempty"`
}

type ServerSpec_Metrics struct {
	// +optional
	// Enabled turns on frps's Prometheus metrics at /metrics on port 7500, behind the dashboard
	// credentials.
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	// ServiceMonitor creates a Prometheus Operator ServiceMonitor. Requires enabled.
	ServiceMonitor bool `json:"serviceMonitor,omitempty"`
}

// ServerStatus is what a private cluster needs to connect.
type ServerStatus struct {
	// +optional
	// Address is the Client's host: the load balancer's IP or hostname, or
	// <server>-frps.<namespace>.svc for a ClusterIP Service.
	Address string `json:"address,omitempty"`
	// +optional
	// ExposedPorts are the expanded allowPorts; copy them into a private ServerPool's allowedPorts.
	ExposedPorts *ServerStatus_ExposedPorts `json:"exposedPorts,omitempty"`
	// +optional
	// ConnectionSecret holds token (and ca.crt); copy it to private clusters.
	ConnectionSecret string `json:"connectionSecret,omitempty"`
	// +optional
	TLS *ServerStatus_TLS `json:"tls,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ServerStatus_ExposedPorts struct {
	// +optional
	TCP []int32 `json:"tcp,omitempty"`
	// +optional
	UDP []int32 `json:"udp,omitempty"`
}

type ServerStatus_TLS struct {
	// +optional
	// ServerNames are the DNS names of bind.tls.certificate; use one as a Client's tls.serverName.
	ServerNames []string `json:"serverNames,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Server is the Schema for the servers API: an frps server run by the operator.
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
