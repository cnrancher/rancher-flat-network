package ip

import (
	"bytes"
	"fmt"
	"net"
	"slices"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/util/retry"
)

func (h *handler) handleIPRemove(s string, ip *flv1.IP) (*flv1.IP, error) {
	if ip == nil || ip.Name == "" {
		return ip, nil
	}

	h.allocateMutex.Lock()
	defer h.allocateMutex.Unlock()

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.subnetCache.Get(flv1.SubnetNamespace, ip.Spec.Subnet)
		if err != nil {
			return fmt.Errorf("failed to get subnet from cache: %w", err)
		}

		result = result.DeepCopy()
		result.Status.UsedIP = ipcalc.RemoveIPFromRange(ip.Status.Address, result.Status.UsedIP)
		result.Status.UsedIPCount--
		if len(ip.Status.MAC) != 0 {
			result.Status.UsedMac = slices.DeleteFunc(result.Status.UsedMac, func(m net.HardwareAddr) bool {
				return bytes.Equal(m, ip.Status.MAC)
			})
		}
		_, err = h.subnetClient.UpdateStatus(result)
		return err
	})
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to remove usedIP & usedMAC from subnet: %v", err)
	}
	if ip.Status.MAC != nil {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] MAC [%v] from subnet [%v]",
				ip.Status.Address, ip.Status.MAC, ip.Spec.Subnet)
	} else {
		logrus.WithFields(fieldsIP(ip)).
			Infof("remove IP [%v] from subnet [%v]",
				ip.Status.Address, ip.Spec.Subnet)
	}

	return ip, nil
}
