package webhook

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

const (
	labelMaster   = "master"
	labelVlan     = "vlan"
	labelMode     = "mode"
	labelFlatMode = "flatMode"
)

func deserializeFlatNetworkSubnet(ar *admissionv1.AdmissionReview) (*flv1.FlatNetworkSubnet, error) {
	/* unmarshal FlatNetworkSubnet from AdmissionReview request */
	subnet := &flv1.FlatNetworkSubnet{}
	err := json.Unmarshal(ar.Request.Object.Raw, subnet)
	return subnet, err
}

func (h *Handler) validateFlatNetworkSubnet(ar *admissionv1.AdmissionReview) (bool, error) {
	subnet, err := deserializeFlatNetworkSubnet(ar)
	if err != nil {
		return false, err
	}

	set := map[string]string{
		labelMaster: subnet.Spec.Master,
		labelVlan:   fmt.Sprintf("%v", subnet.Spec.VLAN),
	}
	subnets, err := h.subnetCache.List(flv1.SubnetNamespace, labels.SelectorFromSet(set))
	if err != nil {
		return false, fmt.Errorf("failed to list subnet by selector %q: %w",
			utils.Print(set), err)
	}
	// Validate subnet spec (CIDR, gw, ranges, routes)
	if err := common.ValidateSubnet(subnet); err != nil {
		return false, err
	}
	// Ensure only one flatMode on iface
	if err := common.CheckSubnetFlatMode(subnet, subnets); err != nil {
		return false, err
	}
	// Ensure no subnet CIDR conflict
	if err := common.CheckSubnetConflict(subnet, subnets); err != nil {
		return false, err
	}
	logrus.Infof("handle flatnetwork subnet validate request [%v]", subnet.Name)
	return true, nil
}
