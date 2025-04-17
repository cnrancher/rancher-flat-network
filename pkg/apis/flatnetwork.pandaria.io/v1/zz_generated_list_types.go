/*
Copyright 2025 SUSE Rancher

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

// Code generated by main. DO NOT EDIT.

// +k8s:deepcopy-gen=package
// +groupName=flatnetwork.pandaria.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlatNetworkIPList is a list of FlatNetworkIP resources
type FlatNetworkIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FlatNetworkIP `json:"items"`
}

func NewFlatNetworkIP(namespace, name string, obj FlatNetworkIP) *FlatNetworkIP {
	obj.APIVersion, obj.Kind = SchemeGroupVersion.WithKind("FlatNetworkIP").ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlatNetworkSubnetList is a list of FlatNetworkSubnet resources
type FlatNetworkSubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FlatNetworkSubnet `json:"items"`
}

func NewFlatNetworkSubnet(namespace, name string, obj FlatNetworkSubnet) *FlatNetworkSubnet {
	obj.APIVersion, obj.Kind = SchemeGroupVersion.WithKind("FlatNetworkSubnet").ToAPIVersionAndKind()
	obj.Name = name
	obj.Namespace = namespace
	return &obj
}
