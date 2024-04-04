//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Client) DeepCopyInto(out *Client) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Client.
func (in *Client) DeepCopy() *Client {
	if in == nil {
		return nil
	}
	out := new(Client)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Client) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientList) DeepCopyInto(out *ClientList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Client, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientList.
func (in *ClientList) DeepCopy() *ClientList {
	if in == nil {
		return nil
	}
	out := new(ClientList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClientList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientSpec) DeepCopyInto(out *ClientSpec) {
	*out = *in
	in.Server.DeepCopyInto(&out.Server)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientSpec.
func (in *ClientSpec) DeepCopy() *ClientSpec {
	if in == nil {
		return nil
	}
	out := new(ClientSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientSpec_Server) DeepCopyInto(out *ClientSpec_Server) {
	*out = *in
	in.Authentication.DeepCopyInto(&out.Authentication)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientSpec_Server.
func (in *ClientSpec_Server) DeepCopy() *ClientSpec_Server {
	if in == nil {
		return nil
	}
	out := new(ClientSpec_Server)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientSpec_Server_Authentication) DeepCopyInto(out *ClientSpec_Server_Authentication) {
	*out = *in
	if in.Token != nil {
		in, out := &in.Token, &out.Token
		*out = new(ClientSpec_Server_Authentication_Token)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientSpec_Server_Authentication.
func (in *ClientSpec_Server_Authentication) DeepCopy() *ClientSpec_Server_Authentication {
	if in == nil {
		return nil
	}
	out := new(ClientSpec_Server_Authentication)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientSpec_Server_Authentication_Token) DeepCopyInto(out *ClientSpec_Server_Authentication_Token) {
	*out = *in
	out.Secret = in.Secret
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientSpec_Server_Authentication_Token.
func (in *ClientSpec_Server_Authentication_Token) DeepCopy() *ClientSpec_Server_Authentication_Token {
	if in == nil {
		return nil
	}
	out := new(ClientSpec_Server_Authentication_Token)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientStatus) DeepCopyInto(out *ClientStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientStatus.
func (in *ClientStatus) DeepCopy() *ClientStatus {
	if in == nil {
		return nil
	}
	out := new(ClientStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Secret) DeepCopyInto(out *Secret) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Secret.
func (in *Secret) DeepCopy() *Secret {
	if in == nil {
		return nil
	}
	out := new(Secret)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Upstream) DeepCopyInto(out *Upstream) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Upstream.
func (in *Upstream) DeepCopy() *Upstream {
	if in == nil {
		return nil
	}
	out := new(Upstream)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Upstream) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamList) DeepCopyInto(out *UpstreamList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Upstream, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamList.
func (in *UpstreamList) DeepCopy() *UpstreamList {
	if in == nil {
		return nil
	}
	out := new(UpstreamList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *UpstreamList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec) DeepCopyInto(out *UpstreamSpec) {
	*out = *in
	if in.TCP != nil {
		in, out := &in.TCP, &out.TCP
		*out = new(UpstreamSpec_TCP)
		(*in).DeepCopyInto(*out)
	}
	if in.UDP != nil {
		in, out := &in.UDP, &out.UDP
		*out = new(UpstreamSpec_UDP)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec.
func (in *UpstreamSpec) DeepCopy() *UpstreamSpec {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec_TCP) DeepCopyInto(out *UpstreamSpec_TCP) {
	*out = *in
	out.Server = in.Server
	if in.ProxyProtocol != nil {
		in, out := &in.ProxyProtocol, &out.ProxyProtocol
		*out = new(string)
		**out = **in
	}
	if in.HealthCheck != nil {
		in, out := &in.HealthCheck, &out.HealthCheck
		*out = new(UpstreamSpec_TCP_HealthCheck)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec_TCP.
func (in *UpstreamSpec_TCP) DeepCopy() *UpstreamSpec_TCP {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec_TCP)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec_TCP_HealthCheck) DeepCopyInto(out *UpstreamSpec_TCP_HealthCheck) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec_TCP_HealthCheck.
func (in *UpstreamSpec_TCP_HealthCheck) DeepCopy() *UpstreamSpec_TCP_HealthCheck {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec_TCP_HealthCheck)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec_TCP_Server) DeepCopyInto(out *UpstreamSpec_TCP_Server) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec_TCP_Server.
func (in *UpstreamSpec_TCP_Server) DeepCopy() *UpstreamSpec_TCP_Server {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec_TCP_Server)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec_UDP) DeepCopyInto(out *UpstreamSpec_UDP) {
	*out = *in
	out.Server = in.Server
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec_UDP.
func (in *UpstreamSpec_UDP) DeepCopy() *UpstreamSpec_UDP {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec_UDP)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamSpec_UDP_Server) DeepCopyInto(out *UpstreamSpec_UDP_Server) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamSpec_UDP_Server.
func (in *UpstreamSpec_UDP_Server) DeepCopy() *UpstreamSpec_UDP_Server {
	if in == nil {
		return nil
	}
	out := new(UpstreamSpec_UDP_Server)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpstreamStatus) DeepCopyInto(out *UpstreamStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpstreamStatus.
func (in *UpstreamStatus) DeepCopy() *UpstreamStatus {
	if in == nil {
		return nil
	}
	out := new(UpstreamStatus)
	in.DeepCopyInto(out)
	return out
}
