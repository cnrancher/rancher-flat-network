package common

import (
	"bytes"
	"fmt"
	"math"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

func ValidateSubnet(subnet *flv1.FlatNetworkSubnet) error {
	switch subnet.Spec.FlatMode {
	case flv1.FlatModeMacvlan:
	case flv1.FlatModeIPvlan:
	default:
		return fmt.Errorf("unrecognized subnet flatMode [%v]", subnet.Spec.FlatMode)
	}

	_, network, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return fmt.Errorf("failed to parse subnet CIDR [%v]: %w",
			subnet.Spec.CIDR, err)
	}

	if len(subnet.Spec.Gateway) != 0 {
		if !network.Contains(subnet.Spec.Gateway) {
			return fmt.Errorf("invalid subnet gateway [%v] provided", subnet.Spec.Gateway)
		}
	}
	if r, err := isValidRanges(subnet.Spec.Ranges, network); err != nil {
		return fmt.Errorf("invalid subnet ranges %v: %w",
			utils.Print(r), err)
	}
	if r, err := isValidRoutes(subnet.Spec.Routes); err != nil {
		return fmt.Errorf("invalid subnet routes %v: %w",
			utils.Print(r), err)
	}

	return nil
}

func isValidRanges(ranges []flv1.IPRange, network *net.IPNet) (*flv1.IPRange, error) {
	if len(ranges) == 0 {
		return nil, nil
	}

	for _, r := range ranges {
		s1 := r.From.To16()
		s2 := r.To.To16()
		if s1 == nil {
			return &r, fmt.Errorf("invalid IP address %q", s1.String())
		}
		if s2 == nil {
			return &r, fmt.Errorf("invalid IP address %q", s2.String())
		}
		if !network.Contains(s1) {
			return &r, fmt.Errorf("addr %q not inside the network %q",
				s1.String(), network.String())
		}
		if !network.Contains(s2) {
			return &r, fmt.Errorf("addr %q not inside the network %q",
				s2.String(), network.String())
		}
		if bytes.Compare(s1, s2) > 0 {
			return &r, fmt.Errorf("invalid subnet range: 'From' should <= 'To'")
		}
	}
	return nil, nil
}

func isValidRoutes(routes []flv1.Route) (*flv1.Route, error) {
	if len(routes) == 0 {
		return nil, nil
	}

	for _, r := range routes {
		if r.Dev == "" {
			return &r, fmt.Errorf("route dev not specified")
		}
		if r.Dst == "" {
			return &r, fmt.Errorf("route dst not specified")
		}
		_, _, err := net.ParseCIDR(r.Dst)
		if err != nil {
			return &r, fmt.Errorf("route dst %q invalid: %w", r.Dst, err)
		}
		if r.Priority < 0 || r.Priority > math.MaxInt32 {
			return &r, fmt.Errorf("invalid route priority (metrics)")
		}
	}
	return nil, nil
}

func CheckSubnetConflict(
	subnet *flv1.FlatNetworkSubnet, subnets []*flv1.FlatNetworkSubnet,
) error {
	if len(subnets) == 0 {
		return nil
	}
	networkIP, _, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return fmt.Errorf("failed to parse CIDR %q: %w", subnet.Spec.CIDR, err)
	}
	for _, s := range subnets {
		if s == nil {
			continue
		}
		if s.Name == subnet.Name {
			continue
		}
		if s.Spec.FlatMode != subnet.Spec.FlatMode {
			continue
		}

		ip, _, err := net.ParseCIDR(s.Spec.CIDR)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q of subnet %q: %w",
				s.Spec.CIDR, s.Name, err)
		}
		if !ip.Equal(networkIP) {
			continue
		}
		// Subnet are using a same network IP, may have a potential conflict
		if len(subnet.Spec.Ranges) == 0 {
			return fmt.Errorf("subnet CIDR conflict: %q already used by subnet [%v]",
				subnet.Spec.CIDR, s.Name)
		}
		if err := ipcalc.CheckIPRangesConflict(subnet.Spec.Ranges, s.Spec.Ranges); err != nil {
			return fmt.Errorf("subnet CIDR conflict: range conflict with subnet [%v]: %w",
				subnet.Name, err)
		}

		if err := ipcalc.CheckNetworkConflict(s.Spec.CIDR, subnet.Spec.CIDR); err != nil {
			return fmt.Errorf("subnet [%v] and [%v] have potential conflicts: %w",
				s.Name, subnet.Name, err)
		}
	}
	return nil
}

func CheckSubnetFlatMode(
	subnet *flv1.FlatNetworkSubnet, subnets []*flv1.FlatNetworkSubnet,
) error {
	// Validate subnet FlatMode
	switch subnet.Spec.FlatMode {
	case flv1.FlatModeIPvlan, flv1.FlatModeMacvlan:
	default:
		return fmt.Errorf("invalid subnet flatMode %q provided, available: [%v, %v]",
			subnet.Spec.FlatMode, flv1.FlatModeMacvlan, flv1.FlatModeIPvlan)
	}

	if len(subnets) == 0 {
		return nil
	}
	for _, s := range subnets {
		if s == nil {
			continue
		}
		if s.Name == subnet.Name {
			continue
		}
		// Check subnets in same VLAN but with different flatMode
		// to avoid Macvlan & IPvlan using the same master iface
		if s.Spec.VLAN != subnet.Spec.VLAN {
			continue
		}
		master := s.Spec.Master
		if s.Spec.VLAN != 0 {
			master = fmt.Sprintf("%v.%v", s.Spec.Master, s.Spec.VLAN)
		}
		if s.Spec.FlatMode != subnet.Spec.FlatMode {
			return fmt.Errorf("subnet [%v] in flatMode [%v] already using master iface [%v]",
				s.Name, s.Spec.FlatMode, master)
		}
		if s.Spec.Mode != subnet.Spec.Mode {
			return fmt.Errorf("subnet [%v] already using %q mode %q on master iface [%v]",
				s.Name, s.Spec.FlatMode, s.Spec.Mode, master)
		}
	}
	return nil
}
