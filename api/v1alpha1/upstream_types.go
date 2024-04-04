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
	Client string            `json:"client"`
	TCP    *UpstreamSpec_TCP `json:"tcp"`
	UDP    *UpstreamSpec_UDP `json:"udp"`
}

type UpstreamSpec_TCP struct {
	Host   string                  `json:"host"`
	Port   int                     `json:"port"`
	Server UpstreamSpec_TCP_Server `json:"server"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	ProxyProtocol *string                       `json:"proxyProtocol"`
	HealthCheck   *UpstreamSpec_TCP_HealthCheck `json:"healthCheck"`
}

type UpstreamSpec_TCP_Server struct {
	Port int `json:"port"`
}

type UpstreamSpec_TCP_HealthCheck struct {
	TimeoutSeconds  int `json:"timeoutSeconds"`
	MaxFailed       int `json:"maxFailed"`
	IntervalSeconds int `json:"intervalSeconds"`
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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
