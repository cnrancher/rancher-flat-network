package common

import (
	"fmt"
	"net"
	"slices"

	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	PodIfaceEth0 = "eth0"
	PodIfaceEth1 = "eth1"
)

// getPodNativeIP returns IP on Pod iface eth0
// NOTE: will return nil if no IP allocated on pod eth0 iface.
func GetPodNativeIP(podNS ns.NetNS, family int) (net.IP, error) {
	var podNativeIP net.IP
	if err := podNS.Do(func(_ ns.NetNS) error {
		link, err := netlink.LinkByName(PodIfaceEth0)
		if err != nil {
			return fmt.Errorf("failed to get iface %q on pod: %w",
				PodIfaceEth0, err)
		}

		addrs, err := netlink.AddrList(link, family)
		if err != nil {
			return fmt.Errorf("failed to list IP addr on pod iface %q: %w",
				PodIfaceEth0, err)
		}

		if len(addrs) == 0 {
			return nil
		}
		for _, a := range addrs {
			if a.IP.IsLinkLocalUnicast() {
				// Skip link local unicast addr
				continue
			}
			podNativeIP = slices.Clone(a.IP)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("getPodNativeIP: %w", err)
	}

	return podNativeIP, nil
}

func AddrListByName(iface string, family int) ([]netlink.Addr, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, fmt.Errorf("addrListByName: link %q not found: %w",
			iface, err)
	}
	addrs, err := netlink.AddrList(link, family)
	if err != nil {
		return nil, fmt.Errorf("addrListByName: failed to list addr: %w", err)
	}
	return addrs, nil
}

func SetPromiscOn(iface string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("setPromiscOn: failed to search iface %q: %w", iface, err)
	}

	if link.Attrs().Promisc == 1 {
		return nil
	}
	if err = netlink.SetPromiscOn(link); err != nil {
		return fmt.Errorf("setPromiscOn failed on iface %q: %w", iface, err)
	}
	logrus.Infof("set promisc on link [%v]", iface)
	return nil
}

// GetVlanIfaceOnHost gets the VLAN interface <ifname>.<vlanID> (eth0.100) on host
// and create if not exists.
func GetVlanIfaceOnHost(
	master string, mtu int, vlanID int,
) (*types100.Interface, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink.LinkList: failed to list links: %w", err)
	}

	ifName := master
	if vlanID != 0 {
		ifName = fmt.Sprintf("%v.%v", master, vlanID)
	}
	for _, l := range links {
		if l.Attrs().Name == ifName {
			iface := &types100.Interface{}
			iface.Name = ifName
			iface.Mac = l.Attrs().HardwareAddr.String()
			return iface, nil
		}
	}
	return createVLANOnHost(master, mtu, ifName, vlanID)
}

// createVLANOnHost creates a VLAN interface <ifname>.<vlanID> (eth0.1) on root
// network namespace.
func createVLANOnHost(
	master string, MTU int, ifName string, vlanID int,
) (*types100.Interface, error) {
	rootNS, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to get root network NS: %w", err)
	}
	defer rootNS.Close()

	m, err := netlink.LinkByName(master)
	if err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to lookup master %q: %v", master, err)
	}

	vlan := &netlink.Vlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:         MTU,
			Name:        ifName,
			ParentIndex: m.Attrs().Index,
			Namespace:   netlink.NsFd(int(rootNS)),
		},
		VlanId: vlanID,
	}
	if err := netlink.LinkAdd(vlan); err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to create VLAN on %q: %w", ifName, err)
	}
	if err := netlink.LinkSetUp(vlan); err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to set vlan iface [%v] status UP: %w",
			ifName, err)
	}
	logrus.Infof("create vlan interface [%v] on host", ifName)

	// Re-fetch vlan to get all properties/attributes
	contVlan, err := netlink.LinkByName(ifName)
	if err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to refetch vlan iface [%v]: %w",
			ifName, err)
	}
	iface := &types100.Interface{
		Name: ifName,
		Mac:  contVlan.Attrs().HardwareAddr.String(),
	}
	return iface, nil
}
