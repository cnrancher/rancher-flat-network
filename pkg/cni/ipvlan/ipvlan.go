package ipvlan

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

func ModeToString(mode netlink.IPVlanMode) (string, error) {
	switch mode {
	case netlink.IPVLAN_MODE_L2:
		return "l2", nil
	case netlink.IPVLAN_MODE_L3:
		return "l3", nil
	case netlink.IPVLAN_MODE_L3S:
		return "l3s", nil
	default:
		return "", fmt.Errorf("unknown ipvlan mode: %q", mode)
	}
}

func ModeFromString(s string) (netlink.IPVlanMode, error) {
	switch s {
	case "", "l2":
		return netlink.IPVLAN_MODE_L2, nil
	case "l3":
		return netlink.IPVLAN_MODE_L3, nil
	case "l3s":
		return netlink.IPVLAN_MODE_L3S, nil
	default:
		return 0, fmt.Errorf("unknown ipvlan mode: %q", s)
	}
}

func CreateIpvlan(conf *types.NetConf, ifName string, netns ns.NetNS) (*types100.Interface, error) {
	ipvlan := &types100.Interface{}

	mode, err := ModeFromString(conf.FlatNetworkConfig.Mode)
	if err != nil {
		return nil, err
	}

	var m netlink.Link
	// TODO:
	// if conf.LinkContNs {
	if false {
		err = netns.Do(func(_ ns.NetNS) error {
			m, err = netlink.LinkByName(conf.FlatNetworkConfig.Master)
			return err
		})
	} else {
		m, err = netlink.LinkByName(conf.FlatNetworkConfig.Master)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %v",
			conf.FlatNetworkConfig.Master, err)
	}

	// due to kernel bug we have to create with tmpname or it might
	// collide with the name on the host and error out
	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}

	mv := &netlink.IPVlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:         conf.FlatNetworkConfig.MTU,
			Name:        tmpName,
			ParentIndex: m.Attrs().Index,
			Namespace:   netlink.NsFd(int(netns.Fd())),
		},
		Mode: mode,
	}

	// TODO: if conf.LinkContNs {
	if false {
		err = netns.Do(func(_ ns.NetNS) error {
			return netlink.LinkAdd(mv)
		})
	} else {
		if err := netlink.LinkAdd(mv); err != nil {
			return nil, fmt.Errorf("failed to create ipvlan: %v", err)
		}
	}

	err = netns.Do(func(_ ns.NetNS) error {
		err := ip.RenameLink(tmpName, ifName)
		if err != nil {
			return fmt.Errorf("failed to rename ipvlan to %q: %v", ifName, err)
		}
		ipvlan.Name = ifName

		// Re-fetch ipvlan to get all properties/attributes
		contIpvlan, err := netlink.LinkByName(ipvlan.Name)
		if err != nil {
			return fmt.Errorf("failed to refetch ipvlan %q: %v", ipvlan.Name, err)
		}
		ipvlan.Mac = contIpvlan.Attrs().HardwareAddr.String()
		ipvlan.Sandbox = netns.Path()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return ipvlan, nil
}
