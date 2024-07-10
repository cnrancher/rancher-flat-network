package commands

import (
	"errors"
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/logger"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/veth"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
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
	if err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		logrus.Infof("request to delete link [%v]", args.IfName)
		if err := ip.DelLinkByName(args.IfName); err != nil {
			if !errors.Is(err, ip.ErrLinkNotFound) {
				logrus.Warnf("failed to delete link %q: %v", args.IfName, err)
				return err
			}
			logrus.Infof("link [%v] already deleted", args.IfName)
		}
		logrus.Infof("done delete link [%v]", args.IfName)

		logrus.Infof("request to delete veth link [%v]", veth.PodVethIface)
		if err := ip.DelLinkByName(veth.PodVethIface); err != nil {
			if !errors.Is(err, ip.ErrLinkNotFound) {
				logrus.Warnf("failed to delete link %q: %v", args.IfName, err)
				return err
			}
			logrus.Infof("link [%v] already deleted", veth.PodVethIface)
		}
		return nil
	}); err != nil {
		// if NetNs is passed down by the Cloud Orchestration Engine, or if it called multiple times
		// so don't return an error if the device is already removed.
		// https://github.com/kubernetes/kubernetes/issues/43014#issuecomment-287164444
		_, ok := err.(ns.NSPathNotExistErr)
		if ok {
			return nil
		}
		return fmt.Errorf("ip del link failed, netns: %v, interface: %v: %w",
			args.Netns, args.IfName, err)
	}

	return nil
}
