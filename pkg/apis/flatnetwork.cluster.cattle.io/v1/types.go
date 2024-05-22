package v1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// const (
// 	MacvlanSubnetNamespace  = "kube-system"
// 	MacvlanAnnotationPrefix = "macvlan.pandaria.cattle.io/"
// 	AnnotationIP            = "macvlan.pandaria.cattle.io/ip"
// 	AnnotationSubnet        = "macvlan.pandaria.cattle.io/subnet"
// 	LabelSubnet             = "macvlan.pandaria.cattle.io/subnet"
// 	AnnotationMac           = "macvlan.pandaria.cattle.io/mac"
// 	LabelSelectedIP         = "macvlan.pandaria.cattle.io/selectedIp"
// 	LabelMultipleIPHash     = "macvlan.pandaria.cattle.io/multipleIpHash"

// 	AnnotationIngress        = "macvlan.panda.io/ingress"
// 	LabelMacvlanIPType       = "macvlan.panda.io/macvlanIpType"
// 	LabelSelectedMac         = "macvlan.panda.io/selectedMac"
// 	AnnotationMacvlanService = "macvlan.panda.io/macvlanService"
// 	LabelProjectID           = "field.cattle.io/projectId"
// 	LabelWorkloadSelector    = "workload.user.cattle.io/workloadselector"
// 	AnnotationIPDelayReuse   = "macvlan.panda.io/ipDelayReuseTimestamp"
// 	FinalizerIPDelayReuse    = "macvlan.panda.io/ipDelayReuse"
// 	AnnotationsIPv6to4       = "macvlan.panda.io/ipv6to4"
// )

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FlatNetworkIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlatNetworkIPSpec   `json:"spec"`
	Status FlatNetworkIPStatus `json:"status"`
}

type FlatNetworkIPSpec struct {
	Subnet string           `json:"subnet"`
	PodID  string           `json:"podId"`
	CIDR   string           `json:"cidr"`
	MAC    net.HardwareAddr `json:"mac"`
}

type FlatNetworkIPStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
}

////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

type FlatNetworkSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlatNetworkSubnetSpec   `json:"spec"`
	Status FlatNetworkSubnetStatus `json:"status"`
}

type FlatNetworkSubnetSpec struct {
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

type FlatNetworkSubnetStatus struct {
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
	Destination net.IP `json:"destination"`
	Gateway     net.IP `json:"gateway,omitempty"`
	Iface       string `json:"iface,omitempty"`
}

type PodDefaultGateway struct {
	Enable      bool   `json:"enable,omitempty"`
	ServiceCIDR string `json:"serviceCIDR,omitempty"`
}
