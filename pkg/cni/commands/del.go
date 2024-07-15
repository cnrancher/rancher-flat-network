package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/logger"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/route"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func Del(args *skel.CmdArgs) error {
	if err := logger.Setup(); err != nil {
		return err
	}
	logrus.Debugf("cmdDel args: %v", utils.Print(args))

	n, err := loadCNINetConf(args.StdinData)
	if err != nil {
		return err
	}
	logrus.Debugf("cniNetConf: %v", utils.Print(n))

	err = ipam.ExecDel(n.IPAM.Type, args.StdinData)
	if err != nil {
		return fmt.Errorf("failed to execute ipam del: type: [%v], config: [%v]: %w",
			n.IPAM.Type, utils.Print(n), err)
	}
	if args.Netns == "" {
		return nil
	}

	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	var addrs []netlink.Addr
	if err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		iface, err := netlink.LinkByName(args.IfName)
		if err != nil {
			if _, ok := err.(netlink.LinkNotFoundError); ok {
				logrus.Infof("link [%v] already deleted", args.IfName)
				return nil
			}
			return fmt.Errorf("failed to lookup %q: %v", args.IfName, err)
		}

		addrs, err = netlink.AddrList(iface, netlink.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("failed to list addrs on %q: %w", args.IfName, err)
		}
		logrus.Infof("request to delete link [%v]", args.IfName)
		if err = netlink.LinkDel(iface); err != nil {
			return fmt.Errorf("failed to delete %q: %v", args.IfName, err)
		}
		logrus.Infof("done delete link [%v]", args.IfName)

		return nil
	}); err != nil {
		return fmt.Errorf("ip del link failed, netns: %v, interface: %v: %w",
			args.Netns, args.IfName, err)
	}

	if len(addrs) == 0 {
		logrus.Infof("skip delete route on host: no addrs on pod %v",
			args.IfName)
		return nil
	}
	for _, a := range addrs {
		if a.IP.IsLinkLocalUnicast() {
			logrus.Infof("skip delete LinkLocalUnicask route on host: %v",
				a.IP)
			continue
		}
		if err := route.DelFlatNetworkRouteFromHost(a.IP); err != nil {
			return fmt.Errorf("failed to delete route [%v] from host: %w",
				a.IP.String(), err)
		}
		logrus.Infof("done delete route %v on route", a.IP)
	}

	return nil
}
