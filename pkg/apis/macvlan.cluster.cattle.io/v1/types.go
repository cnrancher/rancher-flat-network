package v1

import (
	"net"

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

// MacvlanIP is a specification for a macvlan IP resource
type MacvlanIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MacvlanIPSpec   `json:"spec"`
	Status MacvlanIPStatus `json:"status"`
}

// MacvlanIPSpec is the spec for a MacvlanIP resource
type MacvlanIPSpec struct {
	Subnet string           `json:"subnet"`
	PodID  string           `json:"podId"`
	CIDR   string           `json:"cidr"`
	MAC    net.HardwareAddr `json:"mac"`
}

type MacvlanIPStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
}

////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

type MacvlanSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MacvlanSubnetSpec   `json:"spec"`
	Status MacvlanSubnetStatus `json:"status"`
}

type MacvlanSubnetSpec struct {
	Master            string            `json:"master"`
	VLAN              int               `json:"vlan"`
	CIDR              string            `json:"cidr"`
	Mode              string            `json:"mode"`
	Gateway           net.IP            `json:"gateway"`
	Ranges            []IPRange         `json:"ranges"`
	Routes            []Route           `json:"routes,omitempty"`
	PodDefaultGateway PodDefaultGateway `json:"podDefaultGateway,omitempty"`
	IPDelayReuse      int64             `json:"ipDelayReuse,omitempty"`
}

type MacvlanSubnetStatus struct {
	Phase   string             `json:"phase"`
	UsedIP  []IPRange          `json:"usedIP"`
	UsedMac []net.HardwareAddr `json:"usedMac"`

	FailureMessage string `json:"failureMessage"`
}

type IPRange struct {
	RangeStart net.IP `json:"rangeStart"`
	RangeEnd   net.IP `json:"rangeEnd"`
}

type Route struct {
	Dst   net.IP `json:"dst"`
	GW    net.IP `json:"gw,omitempty"`
	Iface string `json:"iface,omitempty"`
}

type PodDefaultGateway struct {
	Enable      bool   `json:"enable,omitempty"`
	ServiceCIDR string `json:"serviceCidr,omitempty"`
}
