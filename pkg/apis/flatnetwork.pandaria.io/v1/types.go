package v1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SubnetNamespace = "cattle-flat-network"

	// Specification for Annotations
	AnnotationPrefix             = "flatnetwork.pandaria.io/"
	AnnotationIP                 = "flatnetwork.pandaria.io/ip"
	AnnotationSubnet             = "flatnetwork.pandaria.io/subnet"
	AnnotationMac                = "flatnetwork.pandaria.io/mac"
	AnnotationIngress            = "flatnetwork.pandaria.io/ingress"
	AnnotationFlatNetworkService = "flatnetwork.pandaria.io/flatNetworkService"
	AnnotationsIPv6to4           = "flatnetwork.pandaria.io/ipv6to4"

	// Specification for Labels
	LabelSelectedIP        = "flatnetwork.pandaria.io/selectedIP"
	LabelSubnet            = "flatnetwork.pandaria.io/subnet"
	LabelFlatNetworkIPType = "flatnetwork.pandaria.io/flatNetworkIPType"
	LabelSelectedMac       = "flatnetwork.pandaria.io/selectedMac"

	LabelWorkloadSelector = "workload.user.cattle.io/workloadselector"
	LabelProjectID        = "field.cattle.io/projectId"

	AllocateModeAuto     = "auto"
	AllocateModeSpecific = "specific"

	FlatModeIPvlan  = "ipvlan"
	FlatModeMacvlan = "macvlan"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlatNetworkIP is a specification for a flat-network FlatNetworkIP resource
type FlatNetworkIP struct {
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
	MACs []string `json:"macs"`

	// PodID is the Pod metadata.UID
	PodID string `json:"podId"`
}

type IPStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`

	// Addr is the allocated IP address.
	Addr net.IP `json:"addr"`

	// MAC is actual allocated MAC address by CNI
	// can be random in auto mode, or specidied by user.
	MAC string `json:"mac"`
}

////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

type FlatNetworkSubnet struct {
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
	//
	// macvlan: 'bridge, vepa, private, passthru' (default 'bridge');
	// ipvlan: 'l2, l3, l3s' (default 'l2');
	Mode string `json:"mode"`

	// Gateway is the gateway of the subnet (optional).
	Gateway net.IP `json:"gateway"`

	// Ranges is the IP range to allocate IP address (optional).
	Ranges []IPRange `json:"ranges,omitempty"`

	// Routes defines the custom routes.
	Routes []Route `json:"routes,omitempty"`

	// RouteSettings provides some advanced options for custom routes.
	RouteSettings RouteSettings `json:"routeSettings,oitempty"`
}

type SubnetStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`

	// Gateway is the gateway of the subnet.
	Gateway net.IP `json:"gateway"`

	// UsedIP is the used IPRange of this subnet.
	UsedIP      []IPRange `json:"usedIP"`
	UsedIPCount int       `json:"usedIPCount"`

	// UsedMAC is the **USER SPECIFIED** used MAC address.
	UsedMAC []string `json:"usedMac"`
}

// Example: ip route add <DST_CIDR> dev <DEV_NAME> via <VIA_GATEWAY_ADDR> src <SRC_ADDR> metrics <PRIORITY>
type Route struct {
	Dev      string `json:"dev"`                // Interface (dev) name
	Dst      string `json:"dst"`                // Dst CIDR
	Src      net.IP `json:"src,omitempty"`      // Src (optional)
	Via      net.IP `json:"via,omitempty"`      // Via (gateway) (optional)
	Priority int    `json:"priority,omitempty"` // Priority (optional)
}

type RouteSettings struct {
	// AddNodeCIDR adds node CIDR route for flat-network pod if enabled.
	AddNodeCIDR bool `json:"addNodeCIDR"`

	// AddPodIPToHost adds pod flat-network IP routes on host if enabled.
	// If true, it will allow node to directly access Pods running on the current node by flat-network IP.
	// If false, node cannot access Pods running on the current node by flat-network IP.
	AddPodIPToHost bool `json:"addPodIPToHost"`

	// FlatNetworkDefaultGateway lets Pod using the flat-network iface as default gateway.
	// NOTE: need to add custom routes (serviceCIDR, clusterCIDR, nodeCIDR)
	// if pod using the flat-network iface as the default gateway.
	//
	// And the pods’ access to other networks will be restricted.
	// For example, Pods cannot directly access the public networks.
	FlatNetworkDefaultGateway bool `json:"flatNetworkDefaultGateway"`
}

// IPRange defines the closed interval [from, to] of IP ranges.
type IPRange struct {
	From net.IP `json:"from"`
	To   net.IP `json:"to"`
}
