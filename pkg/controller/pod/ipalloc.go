package pod

import (
	"bytes"
	"fmt"
	"net"
	"slices"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	corev1 "k8s.io/api/core/v1"
)

// allocateIPModeAuto allocates IP address & MAC address in auto mode
func (h *handler) allocateIPModeAuto(
	subnet *macvlanv1.MacvlanSubnet, annotationMac string,
) (net.IP, net.HardwareAddr, error) {
	h.IPMutex.Lock()
	defer h.IPMutex.Unlock()

	ip, err := ipcalc.GetAvailableIP(subnet.Spec.CIDR, subnet.Spec.Ranges, subnet.Status.UsedIP)
	if err != nil {
		return nil, nil, err
	}
	// Empty annotation mac address.
	if annotationMac == "" {
		return ip, nil, nil
	}
	// Multiple mac address.
	if strings.Contains(annotationMac, "-") {
		macs := strings.Split(strings.TrimSpace(annotationMac), "-")
		for _, v := range macs {
			mac, err := net.ParseMAC(v)
			if err != nil {
				return nil, nil, fmt.Errorf("allocateAutoIP: failed to parse multiple mac addr [%v]: %w", mac, err)
			}
			_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
				return bytes.Compare(a, b)
			})
			if !ok {
				return ip, mac, nil
			}
		}
		return nil, nil, fmt.Errorf("allocateAutoIP: no available unused mac address from [%v]", annotationMac)
	}

	// Single mac address.
	mac, err := net.ParseMAC(annotationMac)
	if err != nil {
		return nil, nil, fmt.Errorf("allocateAutoIP: failed to parse mac addr [%v]: %w", annotationMac, err)
	}
	_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
		return bytes.Compare(a, b)
	})
	if ok {
		return nil, nil, fmt.Errorf("allocateAutoIP: mac address [%v] already in use", annotationMac)
	}
	return ip, mac, nil
}

func (h *handler) allocateIPModeSingle(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, annotationIP, annotationMac string,
) (allocatedIP net.IP, mac net.HardwareAddr, err error) {
	return
}

func (h *handler) allocateIPModeMultiple(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, annotationIP, annotationMac string,
) (allocatedIP net.IP, mac net.HardwareAddr, err error) {
	return
}
