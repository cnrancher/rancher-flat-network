package macvlansubnet

import (
	"fmt"
	"net"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
)

func (h *handler) onMacvlanSubnetRemove(
	_ string, subnet *macvlanv1.MacvlanSubnet,
) (*macvlanv1.MacvlanSubnet, error) {
	if subnet == nil || subnet.Name == "" {
		return subnet, nil
	}

	// List macvlanIPs using this subnet.
	ips, err := h.macvlanIPCache.List("", labels.SelectorFromSet(labels.Set{
		"subnet": subnet.Name,
	}))
	if err != nil {
		return subnet, fmt.Errorf("onMacvlanSubnetRemove: failed to list macvlanIP by subnet [%v] from cache: %w",
			subnet.Name, err)
	}
	if len(ips) != 0 {
		// TODO: add more logics here if needed.

		usedMap := map[string]net.IP{}
		for _, ip := range ips {
			usedMap[ip.Name] = ip.Status.IP
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Warnf("subnet [%v] deleted, but still have following IPs in use: %v",
				subnet.Name, utils.PrintObject(usedMap))
	}

	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("subnet [%v] removed",
			subnet.Name)
	return subnet, nil
}
