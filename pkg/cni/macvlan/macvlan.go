package macvlan

import (
	"fmt"
	"net"
	"strings"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Options struct {
	Mode   string
	Master string
	MTU    int
	IfName string
	NetNS  ns.NetNS
	MAC    net.HardwareAddr
}

func (o *Options) MacvlanMode() netlink.MacvlanMode {
	switch strings.ToLower(o.Mode) {
	case "", "bridge":
		return netlink.MACVLAN_MODE_BRIDGE
	case "private":
		return netlink.MACVLAN_MODE_PRIVATE
	case "vepa":
		return netlink.MACVLAN_MODE_VEPA
	case "passthru":
		return netlink.MACVLAN_MODE_PASSTHRU
	case "source":
		return netlink.MACVLAN_MODE_SOURCE
	default:
		logrus.Warnf("unrecognized macvlan mode: %q", o.Mode)
		return netlink.MACVLAN_MODE_DEFAULT
	}
}

func Create(o *Options) (*types100.Interface, error) {
	mode := o.MacvlanMode()
	master, err := netlink.LinkByName(o.Master)
	if err != nil {
		return nil, fmt.Errorf("macvlan.Create: failed to get master iface %q: %w",
			o.Master, err)
	}

	// Due to kernel bug we have to create with tmpName or it might
	// collide with the name on the host and error out
	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}
	mv := &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:          o.MTU,
			Name:         tmpName,
			ParentIndex:  master.Attrs().Index,
			Namespace:    netlink.NsFd(int(o.NetNS.Fd())),
			HardwareAddr: o.MAC,
		},
		Mode: mode,
	}
	if err := netlink.LinkAdd(mv); err != nil {
		return nil, fmt.Errorf("macvlan.Create: failed to create macvlan: LinkAdd: %w", err)
	}
	logrus.Debugf("created macvlan iface: %v", utils.Print(mv))
	var result *types100.Interface
	if err := o.NetNS.Do(func(_ ns.NetNS) error {
		err := ip.RenameLink(tmpName, o.IfName)
		if err != nil {
			netlink.LinkDel(mv)
			return fmt.Errorf("macvlan.Create: failed to rename macvlan iface name to %q: %w",
				o.IfName, err)
		}
		logrus.Debugf("rename link %v to %v", tmpName, o.IfName)

		// Re-fetch macvlan to get all properties/attributes
		macvlan, err := netlink.LinkByName(o.IfName)
		if err != nil {
			ip.DelLinkByName(o.IfName)
			return fmt.Errorf("macvlan.Create: failed to refetch macvlan iface %q: %w",
				o.IfName, err)
		}
		logrus.Debugf("refetch macvlan link object: %v", utils.Print(macvlan))

		result = &types100.Interface{
			Name:    o.IfName,
			Mac:     macvlan.Attrs().HardwareAddr.String(),
			Sandbox: o.NetNS.Path(),
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func ModeToString(mode netlink.MacvlanMode) (string, error) {
	switch mode {
	case netlink.MACVLAN_MODE_BRIDGE:
		return "bridge", nil
	case netlink.MACVLAN_MODE_PRIVATE:
		return "private", nil
	case netlink.MACVLAN_MODE_VEPA:
		return "vepa", nil
	case netlink.MACVLAN_MODE_PASSTHRU:
		return "passthru", nil
	default:
		return "", fmt.Errorf("unknown macvlan mode: %q", mode)
	}
}

func ModeFromString(s string) (netlink.MacvlanMode, error) {
	switch s {
	case "", "bridge":
		return netlink.MACVLAN_MODE_BRIDGE, nil
	case "private":
		return netlink.MACVLAN_MODE_PRIVATE, nil
	case "vepa":
		return netlink.MACVLAN_MODE_VEPA, nil
	case "passthru":
		return netlink.MACVLAN_MODE_PASSTHRU, nil
	default:
		return 0, fmt.Errorf("unknown macvlan mode: %q", s)
	}
}
