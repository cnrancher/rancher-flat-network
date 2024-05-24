package macvlanip

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

// allocateIPModeAuto allocates IP address & MAC address in auto mode.
func (h *handler) allocateIPModeAuto(
	ip *macvlanv1.MacvlanIP, pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet,
) error {
	h.IPMutex.Lock()
	defer h.IPMutex.Unlock()

	allocatedIP, err := ipcalc.GetAvailableIP(subnet.Spec.CIDR, subnet.Spec.Ranges, subnet.Status.UsedIP)
	if err != nil {
		return err
	}
	annotationMac := ""
	if pod.Annotations != nil {
		annotationMac = pod.Annotations[macvlanv1.AnnotationMac]
	}

	var allocatedMac net.HardwareAddr
	if strings.Contains(annotationMac, "-") {
		// Allocate Mac from multiple mac address.
		macs := strings.Split(strings.TrimSpace(annotationMac), "-")
		for _, v := range macs {
			mac, err := net.ParseMAC(v)
			if err != nil {
				return fmt.Errorf("allocateAutoIP: failed to parse multi-mac addrs [%v]: %w", mac, err)
			}
			_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
				return bytes.Compare(a, b)
			})
			if !ok {
				// Select the unused mac address from multi-mac addresses.
				allocatedMac = mac
				break
			}
		}
		if allocatedMac == nil {
			return fmt.Errorf("allocateAutoIP: no available mac address from [%v]", annotationMac)
		}
	} else {
		// Single mac address.
		mac, err := net.ParseMAC(annotationMac)
		if err != nil {
			return fmt.Errorf("allocateAutoIP: failed to parse mac addr [%v]: %w",
				annotationMac, err)
		}
		_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
			return bytes.Compare(a, b)
		})
		if ok {
			return fmt.Errorf("allocateAutoIP: mac address [%v] already in use", annotationMac)
		}
		allocatedMac = mac
	}

	// Update macvlanIP status
	ip = ip.DeepCopy()
	ip.Status.IP = allocatedIP
	ip.Status.Mac = allocatedMac
	ip, err = h.macvlanIPClient.UpdateStatus(ip)
	if err != nil {
		return fmt.Errorf("allocateAutoIP: failed to update macvlanIP [%v/%v] status: %w",
			ip.Namespace, ip.Name, err)
	}

	// TODO: Update macvlanSubnet status.

	return nil
}

func (h *handler) allocateIPModeSingle(
	ip *macvlanv1.MacvlanIP, pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet,
) error {
	h.IPMutex.Lock()
	defer h.IPMutex.Unlock()

	return nil
}

func (h *handler) allocateIPModeMultiple(
	ip *macvlanv1.MacvlanIP, pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet,
) error {
	h.IPMutex.Lock()
	defer h.IPMutex.Unlock()

	return nil
}
