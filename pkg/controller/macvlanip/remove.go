package macvlanip

import (
	"bytes"
	"fmt"
	"net"
	"slices"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/util/retry"
)

func (h *handler) handleMacvlanIPRemove(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" {
		return ip, nil
	}

	h.allocateMutex.Lock()
	defer h.allocateMutex.Unlock()

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.macvlanSubnetCache.Get(macvlanv1.SubnetNamespace, ip.Spec.Subnet)
		if err != nil {
			return fmt.Errorf("failed to get subnet from cache: %w", err)
		}

		result = result.DeepCopy()
		result.Status.UsedIP = ipcalc.RemoveIPFromRange(ip.Status.IP, result.Status.UsedIP)
		result.Status.UsedIPCount--
		if len(ip.Status.MAC) != 0 {
			result.Status.UsedMac = slices.DeleteFunc(result.Status.UsedMac, func(m net.HardwareAddr) bool {
				return bytes.Equal(m, ip.Status.MAC)
			})
		}
		_, err = h.macvlanSubnetClient.UpdateStatus(result)
		return err
	})
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to remove usedIP & usedMAC from subnet: %v", err)
	}
	if ip.Status.MAC != nil {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] MAC [%v] from subnet [%v]",
				ip.Status.IP, ip.Status.MAC, ip.Spec.Subnet)
	} else {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] from subnet [%v]",
				ip.Status.IP, ip.Spec.Subnet)
	}

	return ip, nil
}
