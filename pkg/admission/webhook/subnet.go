package webhook

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
)

const (
	labelMaster   = "master"
	labelVlan     = "vlan"
	labelMode     = "mode"
	labelFlatMode = "flatMode"

	listInterval = time.Microsecond * 100
	listLimit    = 100
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
	if subnet == nil || subnet.Name == "" || subnet.DeletionTimestamp != nil {
		return true, nil
	}

	var subnets = make([]*flv1.FlatNetworkSubnet, 0)
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%v=%v,%v=%v",
			labelMaster, subnet.Spec.Master, labelVlan, subnet.Spec.VLAN),
		Limit:    listLimit,
		Continue: "",
	}
	for {
		subnetList, err := h.subnetClient.List(flv1.SubnetNamespace, options)
		if err != nil {
			return false, fmt.Errorf("failed to list subnet by selector %q: %w",
				options.LabelSelector, err)
		}
		for i := range subnetList.Items {
			subnets = append(subnets, subnetList.Items[i].DeepCopy())
		}
		if subnetList.Continue == "" {
			break
		}
		options.Continue = subnetList.Continue
		time.Sleep(listInterval)
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
