package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MacvlanSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MacvlanSubnetSpec `json:"spec"`
}

type MacvlanSubnetSpec struct {
	Master            string            `json:"master"`
	VLAN              int               `json:"vlan"`
	CIDR              string            `json:"cidr"`
	Mode              string            `json:"mode"`
	Gateway           string            `json:"gateway"`
	Ranges            []IPRange         `json:"ranges"`
	Routes            []Route           `json:"routes,omitempty"`
	PodDefaultGateway PodDefaultGateway `json:"podDefaultGateway,omitempty"`
	IPDelayReuse      int64             `json:"ipDelayReuse,omitempty"`
}

type IPRange struct {
	RangeStart string `json:"rangeStart"`
	RangeEnd   string `json:"rangeEnd"`
}

type Route struct {
	Dst   string `json:"dst"`
	GW    string `json:"gw,omitempty"`
	Iface string `json:"iface,omitempty"`
}

type PodDefaultGateway struct {
	Enable      bool   `json:"enable,omitempty"`
	ServiceCIDR string `json:"serviceCidr,omitempty"`
}

///////////////////

// MacvlanIP is a specification for a MacvlanIP resource
type MacvlanIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MacvlanIPSpec `json:"spec"`
}

// MacvlanIPSpec is the spec for a MacvlanIP resource
type MacvlanIPSpec struct {
	Subnet string `json:"subnet"`
	PodID  string `json:"podId"`
	CIDR   string `json:"cidr"`
	MAC    string `json:"mac"`
}
