/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpstreamSpec defines the desired state of Upstream
type UpstreamSpec struct {
	Client string `json:"client"`
	// +optional
	TCP *UpstreamSpec_TCP `json:"tcp,omitempty"`
	// +optional
	UDP *UpstreamSpec_UDP `json:"udp,omitempty"`
	// +optional
	STCP *UpstreamSpec_STCP `json:"stcp,omitempty"`
	// +optional
	XTCP *UpstreamSpec_XTCP `json:"xtcp,omitempty"`
	// +optional
	HTTP *UpstreamSpec_HTTP `json:"http,omitempty"`
	// +optional
	HTTPS *UpstreamSpec_HTTPS `json:"https,omitempty"`
	// +optional
	TCPMUX *UpstreamSpec_TCPMUX `json:"tcpmux,omitempty"`
}

// UpstreamSpec_TCPMUX exposes a service using TCP multiplexing over HTTP CONNECT
type UpstreamSpec_TCPMUX struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:Enum=httpconnect
	Multiplexer   string   `json:"multiplexer"`
	CustomDomains []string `json:"customDomains"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}

type UpstreamSpec_STCP struct {
	Host      string                      `json:"host"`
	Port      int                         `json:"port"`
	SecretKey UpstreamSpec_STCP_SecretKey `json:"secretKey"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport"`
	// +optional
	// AllowUsers specifies which FRP users can connect to this tunnel.
	// Use "*" to allow any user. Empty means only the same user.
	AllowUsers []string `json:"allowUsers,omitempty"`
}

type UpstreamSpec_STCP_SecretKey struct {
	Secret Secret `json:"secret"`
}

type UpstreamSpec_XTCP struct {
	Host      string                      `json:"host"`
	Port      int                         `json:"port"`
	SecretKey UpstreamSpec_XTCP_SecretKey `json:"secretKey"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport"`
	// +optional
	// AllowUsers specifies which FRP users can connect to this tunnel.
	// Use "*" to allow any user. Empty means only the same user.
	AllowUsers []string `json:"allowUsers,omitempty"`
}

type UpstreamSpec_XTCP_SecretKey struct {
	Secret Secret `json:"secret"`
}

type UpstreamSpec_HTTP struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +optional
	Subdomain string `json:"subdomain,omitempty"`
	// +optional
	CustomDomains []string `json:"customDomains,omitempty"`
	// +optional
	Locations []string `json:"locations,omitempty"`
	// +optional
	HostHeaderRewrite string `json:"hostHeaderRewrite,omitempty"`
	// +optional
	RequestHeaders *HTTPHeaders `json:"requestHeaders,omitempty"`
	// +optional
	ResponseHeaders *HTTPHeaders `json:"responseHeaders,omitempty"`
	// +optional
	HTTPUser *SecretRef `json:"httpUser,omitempty"`
	// +optional
	HTTPPassword *SecretRef `json:"httpPassword,omitempty"`
	// +optional
	HealthCheck *UpstreamSpec_HTTP_HealthCheck `json:"healthCheck,omitempty"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}

type HTTPHeaders struct {
	Set map[string]string `json:"set,omitempty"`
}

type UpstreamSpec_HTTP_HealthCheck struct {
	// +kubebuilder:validation:Enum=http
	Type            string `json:"type"`
	Path            string `json:"path"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	IntervalSeconds int    `json:"intervalSeconds"`
	MaxFailed       int    `json:"maxFailed"`
}

type UpstreamSpec_HTTPS struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	CustomDomains []string `json:"customDomains"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol,omitempty"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}

// LoadBalancer configures load balancing across multiple upstreams
type LoadBalancer struct {
	// Group is the load balancer group name
	Group string `json:"group"`
	// +optional
	// GroupKey is the shared secret for the group
	GroupKey *SecretRef `json:"groupKey,omitempty"`
}

// UpstreamPlugin configures an FRP plugin instead of direct forwarding
type UpstreamPlugin struct {
	// +kubebuilder:validation:Enum=socks5;http_proxy;static_file;https2http;https2https;http2http;http2https;unix_domain_socket
	Type string `json:"type"`

	// For socks5, http_proxy
	// +optional
	Username *SecretRef `json:"username,omitempty"`
	// +optional
	Password *SecretRef `json:"password,omitempty"`

	// For static_file
	// +optional
	LocalPath string `json:"localPath,omitempty"`
	// +optional
	StripPrefix string `json:"stripPrefix,omitempty"`
	// +optional
	HTTPUser *SecretRef `json:"httpUser,omitempty"`
	// +optional
	HTTPPassword *SecretRef `json:"httpPassword,omitempty"`

	// For https2http, https2https, http2https
	// +optional
	LocalAddr string `json:"localAddr,omitempty"`

	// For unix_domain_socket
	// +optional
	UnixPath string `json:"unixPath,omitempty"`
}

type UpstreamSpec_TCP struct {
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	Port   int                     `json:"port,omitempty"`
	Server UpstreamSpec_TCP_Server `json:"server"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string `json:"proxyProtocol,omitempty"`
	// +optional
	HealthCheck *UpstreamSpec_TCP_HealthCheck `json:"healthCheck,omitempty"`
	// +optional
	Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
	// +optional
	LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`
	// +optional
	Plugin *UpstreamPlugin `json:"plugin,omitempty"`
}

type UpstreamSpec_TCP_Server struct {
	Port int `json:"port"`
}

type UpstreamSpec_TCP_HealthCheck struct {
	TimeoutSeconds  int `json:"timeoutSeconds"`
	MaxFailed       int `json:"maxFailed"`
	IntervalSeconds int `json:"intervalSeconds"`
}

type UpstreamSpec_TCP_Transport struct {
	// +kubebuilder:default=true
	UseEncryption bool `json:"useEncryption"`
	// +kubebuilder:default=false
	UseCompression bool `json:"useCompression"`
	// +optional
	BandwdithLimit *UpstreamSpec_TCP_Transport_BandwdithLimit `json:"bandwidthLimit"`
	// +optional
	ProxyURL *string `json:"proxyURL"`
}

type UpstreamSpec_TCP_Transport_BandwdithLimit struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
	// +kubebuilder:validation:Enum=KB;MB
	Type string `json:"type"`
}

type UpstreamSpec_UDP struct {
	Host   string                  `json:"host"`
	Port   int                     `json:"port"`
	Server UpstreamSpec_UDP_Server `json:"server"`
}

type UpstreamSpec_UDP_Server struct {
	Port int `json:"port"`
}

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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Client",type=string,JSONPath=`.spec.client`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Upstream is the Schema for the upstreams API
type Upstream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpstreamSpec   `json:"spec,omitempty"`
	Status UpstreamStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UpstreamList contains a list of Upstream
type UpstreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Upstream `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Upstream{}, &UpstreamList{})
}
