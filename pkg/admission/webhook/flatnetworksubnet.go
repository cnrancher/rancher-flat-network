package webhook

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"

	flatnetworkv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
)

func deserializeMacvlanSubnet(ar *admissionv1.AdmissionReview) (flatnetworkv1.FlatNetworkSubnet, error) {
	/* unmarshal FlatNetworkSubnet from AdmissionReview request */
	subnet := flatnetworkv1.FlatNetworkSubnet{}
	err := json.Unmarshal(ar.Request.Object.Raw, &subnet)
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
	if err := h.checkSubnetConflict(subnet); err != nil {
		return false, err
	}
	return true, nil
}

func validateSubnetRouteGateway(subnet flatnetworkv1.FlatNetworkSubnet) error {
	_, ipnet, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return err
	}
	for _, v := range subnet.Spec.Routes {
		if v.Interface == "eth1" || v.Interface == "" {
			if len(v.Gateway) == 0 {
				continue
			}
			ip := slices.Clone(v.Gateway)
			if !ipnet.Contains(ip) {
				return fmt.Errorf("invalid gateway ip '%s' is not in network '%s'", v.Gateway, subnet.Spec.CIDR)
			}
		}
	}
	return nil
}

func (h *Handler) checkSubnetConflict(subnet flatnetworkv1.FlatNetworkSubnet) error {
	subnets, err := h.subnetCache.List(flatnetworkv1.SubnetNamespace, labels.SelectorFromSet(map[string]string{
		"vlan": fmt.Sprintf("%v", subnet.Spec.VLAN),
	}))
	if err != nil {
		return err
	}

	for _, s := range subnets {
		if s.Name == subnet.Name {
			continue
		}
		if err := ipcalc.CheckIPRangesConflict(s.Spec.Ranges, subnet.Spec.Ranges); err != nil {
			return fmt.Errorf("subnet [%v] and [%v] have potential conflicts: %w",
				s.Name, subnet.Name, err)
		}
		if err := ipcalc.CheckNetworkConflict(s.Spec.CIDR, subnet.Spec.CIDR); err != nil {
			return fmt.Errorf("subnet [%v] and [%v] have potential conflicts: %w",
				s.Name, subnet.Name, err)
		}
	}
	return nil
}
