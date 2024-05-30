package macvlanip

import (
	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
)

func (h *handler) handleMacvlanIPRemove(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" {
		return ip, nil
	}

	logrus.WithFields(fieldsIP(ip)).Infof("macvlanIP deleted.")

	return ip, nil
}
