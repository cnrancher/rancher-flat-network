package flatnetworksubnet

import (
	"fmt"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
)

func (h *handler) handleSubnetRemove(
	_ string, subnet *flv1.FlatNetworkSubnet,
) (*flv1.FlatNetworkSubnet, error) {
	if subnet == nil || subnet.Name == "" {
		return subnet, nil
	}

	// List IPs using this subnet.
	ips, err := h.ipCache.List("", labels.SelectorFromSet(labels.Set{
		"subnet": subnet.Name,
	}))
	if err != nil {
		return subnet, fmt.Errorf("handleSubnetRemove: failed to list IP by subnet [%v] from cache: %w",
			subnet.Name, err)
	}
	if len(ips) != 0 {
		// TODO: add more logics here if needed.

		usedMap := map[string]net.IP{}
		for _, ip := range ips {
			usedMap[ip.Name] = ip.Status.Addr
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Warnf("subnet [%v] deleted, but still have following IPs in use: %v",
				subnet.Name, utils.Print(usedMap))
	}

	subnet = subnet.DeepCopy()
	subnet.Status.Phase = ""
	subnetUpdate, err := h.subnetClient.UpdateStatus(subnet)
	if err != nil {
		return subnet, err
	}
	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("subnet [%v] removed",
			subnetUpdate.Name)
	return subnetUpdate, nil
}
