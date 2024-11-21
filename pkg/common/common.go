package common

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/ipvlan"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/macvlan"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

const (
	KindDeployment  = "Deployment"
	KindDaemonSet   = "DaemonSet"
	KindStatefulSet = "StatefulSet"
	KindCronJob     = "CronJob"
	KindJob         = "Job"
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
	if r, err := isValidRoutes(network, subnet.Spec.Routes); err != nil {
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

func isValidRoutes(network *net.IPNet, routes []flv1.Route) (*flv1.Route, error) {
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
		if len(r.Via) != 0 {
			if !network.Contains(r.Via) {
				return &r, fmt.Errorf("invalid gateway ip %q: not in subnet CIDR", r.Via)
			}
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
	for _, s := range subnets {
		if s == nil {
			continue
		}
		if s.Name == subnet.Name {
			continue
		}
		if s.Spec.FlatMode != subnet.Spec.FlatMode {
			continue // skip using different flatMode
		}
		if s.Spec.VLAN != subnet.Spec.VLAN {
			continue // skip using different VLAN
		}
		if err := ipcalc.CheckNetworkConflict(s.Spec.CIDR, subnet.Spec.CIDR); err != nil {
			if len(s.Spec.Ranges) != 0 && len(subnet.Spec.Ranges) != 0 {
				err := ipcalc.CheckIPRangesConflict(subnet.Spec.Ranges, s.Spec.Ranges)
				if err != nil {
					// Subnets using same CIDR and with conflict ranges,
					// return ip range conflicts.
					return fmt.Errorf("range conflict with subnet [%v]: %w",
						s.Name, err)
				}
				// Subnets using the same CIDR but different ranges, pass.
				return nil
			}

			// Subnet using same CIDR and not providing ranges,
			// return CIDR conflict.
			return fmt.Errorf("subnet [%v] and [%v] have potential CIDR conflicts: %w",
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
	case flv1.FlatModeMacvlan:
		_, err := macvlan.ModeFromString(subnet.Spec.Mode)
		if err != nil {
			return fmt.Errorf("invalid %q mode %q: %w",
				subnet.Spec.FlatMode, subnet.Spec.Mode, err)
		}
		if subnet.Spec.IPvlanFlag != "" {
			return fmt.Errorf("ipvlanFlag should be empty when flatMode is %q",
				subnet.Spec.FlatMode)
		}
	case flv1.FlatModeIPvlan:
		_, err := ipvlan.ModeFromString(subnet.Spec.Mode)
		if err != nil {
			return fmt.Errorf("invalid %q mode %q: %w",
				subnet.Spec.FlatMode, subnet.Spec.Mode, err)
		}
		_, err = ipvlan.FlagFromString(subnet.Spec.IPvlanFlag)
		if err != nil {
			return fmt.Errorf("invalid %q flag %q: %w",
				subnet.Spec.FlatMode, subnet.Spec.IPvlanFlag, err)
		}
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
		// Linux allows different macvlan/ipvlan modes on same master iface
		// if s.Spec.Mode != subnet.Spec.Mode {
		// 	return fmt.Errorf("subnet [%v] already using %q mode %q on master iface [%v]",
		// 		s.Name, s.Spec.FlatMode, s.Spec.Mode, master)
		// }
	}
	return nil
}

func CheckPodAnnotationIPs(s string) ([]net.IP, error) {
	ret := []net.IP{}
	if s == "" || s == flv1.AllocateModeAuto {
		return ret, nil
	}
	ip := net.ParseIP(s)
	if ip != nil {
		return append(ret, ip), nil
	}

	spec := strings.Split(strings.TrimSpace(s), "-")
	if len(spec) == 0 {
		return nil, fmt.Errorf("invalid annotation IP list [%v], should separated by comma", s)
	}
	for _, v := range spec {
		ip := net.ParseIP(v)
		if len(ip) == 0 {
			return nil, fmt.Errorf("invalid annotation IP list [%v]: invalid IP format", v)
		}
		ret = append(ret, ip)
	}
	return ret, nil
}

func CheckPodAnnotationMACs(s string) ([]string, error) {
	ret := []string{}
	if s == "" || s == flv1.AllocateModeAuto {
		return ret, nil
	}

	spec := strings.Split(strings.TrimSpace(s), "-")
	for _, v := range spec {
		m, err := net.ParseMAC(v)
		if err != nil {
			return nil, fmt.Errorf("invalid mac [%v] found in annotation [%v]: %w",
				v, s, err)
		}
		ret = append(ret, m.String())
	}
	return ret, nil
}

func GetWorkloadKind(w metav1.Object) string {
	switch w.(type) {
	case *appsv1.Deployment:
		return KindDeployment
	case *appsv1.DaemonSet:
		return KindDaemonSet
	case *appsv1.StatefulSet:
		return KindStatefulSet
	case *batchv1.CronJob:
		return KindCronJob
	case *batchv1.Job:
		return KindJob
	}
	return ""
}

func GetWorkloadReservdIPKey(w metav1.Object) string {
	k := GetWorkloadKind(w)
	if k == "" {
		// Not a workload
		return ""
	}
	// <Kind>/<Namespace>/<Name>
	return fmt.Sprintf("%s/%s/%s",
		k, w.GetNamespace(), w.GetName())
}
