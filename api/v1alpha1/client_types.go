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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClientSpec defines the desired state of Client
type ClientSpec struct {
	Server ClientSpec_Server `json:"server"`
	// +optional
	// PodTemplate allows customization of the FRP client pod
	PodTemplate *ClientSpec_PodTemplate `json:"podTemplate,omitempty"`
}

type ClientSpec_Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	Protocol       *string                          `json:"protocol,omitempty"`
	Authentication ClientSpec_Server_Authentication `json:"authentication"`
	AdminServer    *ClientSpec_Server_AdminServer   `json:"adminServer,omitempty"`
	// +optional
	STUNServer *string `json:"stunServer,omitempty"`
	// +optional
	// TLS configures TLS client certificate authentication
	TLS *ClientSpec_Server_TLS `json:"tls,omitempty"`
	// +optional
	// Transport configures connection behavior
	Transport *ClientSpec_Server_Transport `json:"transport,omitempty"`
}

// ClientSpec_Server_Transport configures connection behavior for performance tuning
type ClientSpec_Server_Transport struct {
	// +optional
	// PoolCount is the number of pre-established connections to the server
	PoolCount int `json:"poolCount,omitempty"`
	// +optional
	// TCPMux enables TCP stream multiplexing to reduce connection overhead
	TCPMux *bool `json:"tcpMux,omitempty"`
	// +optional
	// DialServerTimeout is the connection timeout to the FRP server
	DialServerTimeout string `json:"dialServerTimeout,omitempty"`
	// +optional
	// DialServerKeepalive is the keepalive interval (-1s to disable)
	DialServerKeepalive string `json:"dialServerKeepalive,omitempty"`
	// +optional
	// ConnectServerLocalIP binds the outbound connection to a specific local IP
	ConnectServerLocalIP string `json:"connectServerLocalIP,omitempty"`
}

type ClientSpec_Server_TLS struct {
	// +kubebuilder:default=true
	// Enable enables TLS for the connection to the FRP server
	Enable bool `json:"enable"`
	// +optional
	// CertFile is a reference to the client certificate
	CertFile *SecretRef `json:"certFile,omitempty"`
	// +optional
	// KeyFile is a reference to the client private key
	KeyFile *SecretRef `json:"keyFile,omitempty"`
	// +optional
	// TrustedCAFile is a reference to the CA certificate for server verification
	TrustedCAFile *ConfigMapOrSecretRef `json:"trustedCaFile,omitempty"`
}

type ClientSpec_Server_Authentication struct {
	// +optional
	// Token authentication using a shared secret
	Token *ClientSpec_Server_Authentication_Token `json:"token,omitempty"`
	// +optional
	// OIDC authentication for enterprise SSO
	OIDC *ClientSpec_Server_Authentication_OIDC `json:"oidc,omitempty"`
}

type ClientSpec_Server_Authentication_Token struct {
	Secret Secret `json:"secret"`
}

type ClientSpec_Server_Authentication_OIDC struct {
	// ClientID is the OIDC client identifier
	ClientID SecretRef `json:"clientId"`
	// ClientSecret is the OIDC client secret
	ClientSecret SecretRef `json:"clientSecret"`
	// TokenEndpointURL is the URL to obtain the access token
	TokenEndpointURL string `json:"tokenEndpointUrl"`
	// +optional
	// Audience is the intended audience of the token
	Audience string `json:"audience,omitempty"`
	// +optional
	// Scope specifies the requested scopes
	Scope string `json:"scope,omitempty"`
}

type ClientSpec_Server_AdminServer struct {
	Port     int                                     `json:"port"`
	Username *ClientSpec_Server_AdminServer_Username `json:"username"`
	Password *ClientSpec_Server_AdminServer_Password `json:"password"`
	// +optional
	// +kubebuilder:default=false
	// PprofEnable enables pprof debug endpoints on the admin server
	PprofEnable bool `json:"pprofEnable,omitempty"`
}

type ClientSpec_Server_AdminServer_Username struct {
	Secret Secret `json:"secret"`
}

type ClientSpec_Server_AdminServer_Password struct {
	Secret Secret `json:"secret"`
}

// ClientSpec_PodTemplate allows customization of the FRP client pod
type ClientSpec_PodTemplate struct {
	// +optional
	// Resources defines compute resources for the FRP client container
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	// NodeSelector is a selector which must match a node's labels for the pod to be scheduled
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	// Tolerations are tolerations for the pod
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	// Affinity is the pod's scheduling constraints
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
	// ImagePullSecrets are references to secrets for pulling the FRP image
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// +optional
	// PriorityClassName is the name of the PriorityClass for the pod
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// +optional
	// SecurityContext holds pod-level security attributes
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
}

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

//+kubebuilder:object:root=true

// ClientList contains a list of Client
type ClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Client `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Client{}, &ClientList{})
}
