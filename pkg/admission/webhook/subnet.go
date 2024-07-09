package webhook

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/ipcalc"
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
)

func deserializeMacvlanSubnet(ar *admissionv1.AdmissionReview) (*flv1.FlatNetworkSubnet, error) {
	/* unmarshal FlatNetworkSubnet from AdmissionReview request */
	subnet := &flv1.FlatNetworkSubnet{}
	err := json.Unmarshal(ar.Request.Object.Raw, subnet)
	return subnet, err
}

func (h *Handler) validateMacvlanSubnet(ar *admissionv1.AdmissionReview) (bool, error) {
	subnet, err := deserializeMacvlanSubnet(ar)
	if err != nil {
		return false, err
	}
	if err := validateSubnetRouteGateway(subnet); err != nil {
		return false, err
	}

	subnets, err := h.subnetCache.List(flv1.SubnetNamespace, labels.SelectorFromSet(map[string]string{
		"vlan": fmt.Sprintf("%v", subnet.Spec.VLAN),
	}))
	if err != nil {
		return false, fmt.Errorf("failed to list subnet: %w", err)
	}
	if err := checkSubnetConflict(subnet, subnets); err != nil {
		return false, err
	}
	if err := checkSubnetFlatMode(subnet, subnets); err != nil {
		return false, err
	}
	logrus.Infof("handle subnet validate request [%v]", subnet.Name)
	return true, nil
}

func validateSubnetRouteGateway(subnet *flv1.FlatNetworkSubnet) error {
	_, ipnet, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return fmt.Errorf("failed to parse subnet CIDR %q: %w", subnet.Spec.CIDR, err)
	}
	for _, v := range subnet.Spec.Routes {
		if v.Iface == "eth1" || v.Iface == "" {
			if len(v.GW) == 0 {
				continue
			}
			ip := slices.Clone(v.GW)
			if !ipnet.Contains(ip) {
				return fmt.Errorf("invalid gateway ip '%s' is not in network '%s'", v.GW, subnet.Spec.CIDR)
			}
		}
	}
	return nil
}

func checkSubnetConflict(
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
		if err := ipcalc.CheckIPRangesConflict(s.Spec.Ranges, subnet.Spec.Ranges); err != nil {
			return fmt.Errorf("iprange in subnet [%v] and [%v] have potential conflicts: %w",
				s.Name, subnet.Name, err)
		}
		if err := ipcalc.CheckNetworkConflict(s.Spec.CIDR, subnet.Spec.CIDR); err != nil {
			return fmt.Errorf("subnet [%v] and [%v] have potential conflicts: %w",
				s.Name, subnet.Name, err)
		}
	}
	return nil
}

func checkSubnetFlatMode(
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
		if s.Spec.FlatMode != subnet.Spec.FlatMode {
			return fmt.Errorf("subnet [%v] in flatMode [%v] already using master iface [%v]",
				s.Name, s.Spec.FlatMode, s.Spec.Master)
		}
	}
	return nil
}
