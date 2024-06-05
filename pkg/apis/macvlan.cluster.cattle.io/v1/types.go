package v1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SubnetNamespace = "kube-system"

	// Specification for Annotations
	AnnotationPrefix         = "macvlan.pandaria.cattle.io/"
	AnnotationIP             = "macvlan.pandaria.cattle.io/ip"
	AnnotationSubnet         = "macvlan.pandaria.cattle.io/subnet"
	AnnotationMac            = "macvlan.pandaria.cattle.io/mac"
	AnnotationIngress        = "macvlan.panda.io/ingress"
	AnnotationMacvlanService = "macvlan.panda.io/macvlanService"
	AnnotationsIPv6to4       = "macvlan.panda.io/ipv6to4"

	// Deprecated: no longer used
	AnnotationIPDelayReuse = "macvlan.panda.io/ipDelayReuseTimestamp"

	// Specification for Labels
	LabelSelectedIP       = "macvlan.pandaria.cattle.io/selectedIp"
	LabelMultipleIPHash   = "macvlan.pandaria.cattle.io/multipleIpHash"
	LabelSubnet           = "macvlan.pandaria.cattle.io/subnet"
	LabelMacvlanIPType    = "macvlan.panda.io/macvlanIpType"
	LabelSelectedMac      = "macvlan.panda.io/selectedMac"
	LabelProjectID        = "field.cattle.io/projectId"
	LabelWorkloadSelector = "workload.user.cattle.io/workloadselector"

	FinalizerIPDelayReuse = "macvlan.panda.io/ipDelayReuse"
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
	// Subnet is the name of the macvlan subnet resource (required).
	Subnet string `json:"subnet"`

	// CIDR is the mode to allocate IP address.
	//
	// Available:
	//   'auto': Allocate one IP address from subnet automatically (default).
	//   '<ip-address>': Use one custom IPv4 address, format '192.168.1.2'.
	//   '<ip-address>/<mask-size>': Use one custom IPv4 address, format '192.168.1.2/24'.
	//   '<ip1>-<ip2>-...-<ipN>': Use multiple custom IPv4 address.
	CIDR string `json:"ip"`

	// MAC is the mode to specify custom MAC address.
	//
	// Available:
	//   '' (empty string): Do not use custom MAC address.
	//   '<mac-address>': Use one custom IPv4 address, format 'aa:bb:cc:dd:ee:ff'.
	//   '<mac1>-<mac2>-...-<macN>': Use multiple custom MAC address.
	MAC string `json:"mac"`

	// PodID is the Pod metadata.UID
	PodID string `json:"podId"`
}

type MacvlanIPStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`

	// IP is the allocated IP address.
	IP net.IP `json:"ip"`

	// MAC is the allocated (user specified only) MAC addr
	MAC net.HardwareAddr `json:"mac"`
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
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`

	// Gateway is the gateway of the subnet.
	Gateway net.IP `json:"gateway"`

	// UsedIP is the used IPRange of this subnet.
	UsedIP      []IPRange `json:"usedIP"`
	UsedIPCount int       `json:"usedIPCount"`

	// UsedMac is the **USER SPECIFIED** used Mac address.
	UsedMac []net.HardwareAddr `json:"usedMac"`
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
