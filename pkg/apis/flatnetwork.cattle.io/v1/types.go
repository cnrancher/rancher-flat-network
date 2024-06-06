package v1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SubnetNamespace = "kube-system"

	// Specification for Annotations
	AnnotationPrefix             = "flatnetwork.cattle.io/"
	AnnotationIP                 = "flatnetwork.cattle.io/ip"
	AnnotationSubnet             = "flatnetwork.cattle.io/subnet"
	AnnotationMac                = "flatnetwork.cattle.io/mac"
	AnnotationIngress            = "flatnetwork.cattle.io/ingress"
	AnnotationFlatNetworkService = "flatnetwork.cattle.io/flatNetworkService"
	AnnotationsIPv6to4           = "flatnetwork.cattle.io/ipv6to4"

	// Specification for Labels
	LabelSelectedIP        = "flatnetwork.cattle.io/selectedIP"
	LabelMultipleIPHash    = "flatnetwork.cattle.io/multipleIPHash"
	LabelSubnet            = "flatnetwork.cattle.io/subnet"
	LabelFlatNetworkIPType = "flatnetwork.cattle.io/flatNetworkIPType"
	LabelSelectedMac       = "flatnetwork.cattle.io/selectedMac"

	LabelWorkloadSelector = "workload.user.cattle.io/workloadselector"
	LabelProjectID        = "field.cattle.io/projectId"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IP is a specification for a flat-network IP resource
type IP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPSpec   `json:"spec"`
	Status IPStatus `json:"status"`
}

// IPSpec is the spec for a IP resource
type IPSpec struct {
	// Subnet is the name of the flat-network subnet resource (required).
	Subnet string `json:"subnet"`

	// Addrs is the user specified IP addresses (optional).
	Addrs []net.IP `json:"addrs"`

	// MACs is the user specified MAC addresses (optional).
	MACs []net.HardwareAddr `json:"macs"`

	// PodID is the Pod metadata.UID
	PodID string `json:"podId"`
}

type IPStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`

	// Addr is the allocated IP address.
	Addr net.IP `json:"addr"`

	// MAC is the allocated (user specified only) MAC addr
	MAC net.HardwareAddr `json:"mac"`
}

////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec"`
	Status SubnetStatus `json:"status"`
}

type SubnetSpec struct {
	// FlatMode is the mode of the flat-network, can be 'macvlan', 'ipvlan'
	FlatMode string `json:"flatMode"`

	// Master is the network interface name.
	Master string `json:"master"`

	// VLAN is the VLAN ID of this subnet.
	VLAN int `json:"vlan"`

	// CIDR is a IPv4/IPv6 network CIDR block of this subnet.
	CIDR string `json:"cidr"`

	// Mode is the network mode for macvlan/ipvlan.
	// Should be 'bridge'.
	Mode string `json:"mode"`

	// Gateway is the gateway of the subnet (optional).
	Gateway net.IP `json:"gateway"`

	// Ranges is the IP range to allocate IP address (optional).
	Ranges []IPRange `json:"ranges"`

	// Routes defines the custom routes.
	Routes []Route `json:"routes,omitempty"`

	// PodDefaultGateway let pod use the flat-network interface as the
	// default gateway.
	PodDefaultGateway PodDefaultGateway `json:"podDefaultGateway,omitempty"`
}

type SubnetStatus struct {
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

type Route struct {
	Destination net.IP `json:"destination"`
	Gateway     net.IP `json:"gateway,omitempty"`
	Interface   string `json:"interface,omitempty"`
}

type PodDefaultGateway struct {
	Enable      bool   `json:"enable,omitempty"`
	ServiceCIDR string `json:"serviceCIDR,omitempty"`
}

type IPRange struct {
	Start net.IP `json:"start"`
	End   net.IP `json:"end"`
}
