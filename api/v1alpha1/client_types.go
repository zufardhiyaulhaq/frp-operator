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

// ClientSpec defines the desired state of Client
type ClientSpec struct {
	Server ClientSpec_Server `json:"server"`
}

type ClientSpec_Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	Protocol       *string                          `json:"protocol"`
	Authentication ClientSpec_Server_Authentication `json:"authentication"`
	AdminServer    *ClientSpec_Server_AdminServer   `json:"adminServer,omitempty"`
	// +optional
	STUNServer *string `json:"stunServer"`
}

type ClientSpec_Server_Authentication struct {
	Token *ClientSpec_Server_Authentication_Token `json:"token"`
}

type ClientSpec_Server_Authentication_Token struct {
	Secret Secret `json:"secret"`
}

type ClientSpec_Server_AdminServer struct {
	Port     int                                     `json:"port"`
	Username *ClientSpec_Server_AdminServer_Username `json:"username"`
	Password *ClientSpec_Server_AdminServer_Password `json:"password"`
}

type ClientSpec_Server_AdminServer_Username struct {
	Secret Secret `json:"secret"`
}

type ClientSpec_Server_AdminServer_Password struct {
	Secret Secret `json:"secret"`
}

// ClientStatus defines the observed state of Client
type ClientStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
