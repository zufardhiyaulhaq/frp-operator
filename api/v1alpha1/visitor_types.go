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

// VisitorSpec defines the desired state of Visitor
type VisitorSpec struct {
	Client string `json:"client"`
	// +optional
	STCP *VisitorSpec_STCP `json:"stcp"`
	// +optional
	XTCP *VisitorSpec_XTCP `json:"xtcp"`
}

type VisitorSpec_STCP struct {
	Host            string                           `json:"host"`
	Port            int                              `json:"port"`
	ServerName      string                           `json:"serverName"`
	ServerSecretKey VisitorSpec_STCP_ServerSecretKey `json:"serverSecretKey"`
}

type VisitorSpec_STCP_ServerSecretKey struct {
	Secret Secret `json:"secret"`
}

type VisitorSpec_XTCP struct {
	Host                 string                           `json:"host"`
	Port                 int                              `json:"port"`
	ServerName           string                           `json:"serverName"`
	ServerSecretKey      VisitorSpec_XTCP_ServerSecretKey `json:"serverSecretKey"`
	Fallback             *VisitorSpec_Fallback            `json:"fallback,omitempty"`
	PersistantConnection bool                             `json:"persistantConnection,omitempty"`
}

type VisitorSpec_XTCP_ServerSecretKey struct {
	Secret Secret `json:"secret"`
}

type VisitorSpec_Fallback struct {
	ServerName string `json:"serverName"`
	Timeout    int    `json:"timeout"`
}

// VisitorStatus defines the observed state of Visitor
type VisitorStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Visitor is the Schema for the visitors API
type Visitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VisitorSpec   `json:"spec,omitempty"`
	Status VisitorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VisitorList contains a list of Visitor
type VisitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Visitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Visitor{}, &VisitorList{})
}
