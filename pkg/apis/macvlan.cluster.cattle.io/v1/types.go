package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MacvlanSubnetNamespace  = "kube-system"
	MacvlanAnnotationPrefix = "macvlan.pandaria.cattle.io/"
	AnnotationIP            = "macvlan.pandaria.cattle.io/ip"
	AnnotationSubnet        = "macvlan.pandaria.cattle.io/subnet"
	LabelSubnet             = "macvlan.pandaria.cattle.io/subnet"
	AnnotationMac           = "macvlan.pandaria.cattle.io/mac"
	LabelSelectedIP         = "macvlan.pandaria.cattle.io/selectedIp"
	LabelMultipleIPHash     = "macvlan.pandaria.cattle.io/multipleIpHash"

	AnnotationIngress        = "macvlan.panda.io/ingress"
	LabelMacvlanIPType       = "macvlan.panda.io/macvlanIpType"
	LabelSelectedMac         = "macvlan.panda.io/selectedMac"
	AnnotationMacvlanService = "macvlan.panda.io/macvlanService"
	LabelProjectID           = "field.cattle.io/projectId"
	LabelWorkloadSelector    = "workload.user.cattle.io/workloadselector"
	AnnotationIPDelayReuse   = "macvlan.panda.io/ipDelayReuseTimestamp"
	FinalizerIPDelayReuse    = "macvlan.panda.io/ipDelayReuse"
	AnnotationsIPv6to4       = "macvlan.panda.io/ipv6to4"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`
// +kubebuilder:printcolumn:name="PodId",type=string,JSONPath=`.spec.podId`
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.spec.mac`

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

////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="Master",type=string,JSONPath=`.spec.master`
// +kubebuilder:printcolumn:name="VLAN",type=string,JSONPath=`.spec.vlan`
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.gateway`

// MacvlanSubnet is a specification for a MacvlanSubnet resource
type MacvlanSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MacvlanSubnetSpec `json:"spec"`
}

// MacvlanSubnetSpec is the spec for a MacvlanSubnet resource
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

// IPRange is a list of ip start end range
type IPRange struct {
	RangeStart string `json:"rangeStart"`
	RangeEnd   string `json:"rangeEnd"`
}

// Route is route option spec
type Route struct {
	Dst   string `json:"dst"`
	GW    string `json:"gw,omitempty"`
	Iface string `json:"iface,omitempty"`
}

type PodDefaultGateway struct {
	Enable      bool   `json:"enable,omitempty"`
	ServiceCIDR string `json:"serviceCidr,omitempty"`
}
