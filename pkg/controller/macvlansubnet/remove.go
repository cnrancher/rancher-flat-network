package macvlansubnet

import (
	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

func (h *handler) handleSubnetRemoved(s string, subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	if subnet == nil || subnet.Name == "" {
		return subnet, nil
	}

	return subnet, nil
}
