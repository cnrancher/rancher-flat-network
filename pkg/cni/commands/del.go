package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
)

func Del(args *skel.CmdArgs) error {
	logrus.Debugf("cmdDel args: %v", utils.Print(args))

	n, err := loadCNINetConf(args.StdinData)
	if err != nil {
		return err
	}

	err = ipam.ExecDel(n.FlatNetworkConfig.IPAM.Type, args.StdinData)
	if err != nil {
		return fmt.Errorf("failed to execute ipam del: type: [%v], config: [%v]: %w",
			n.FlatNetworkConfig.IPAM.Type, utils.Print(n), err)
	}

	if args.Netns == "" {
		return nil
	}

	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	if err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		if err := ip.DelLinkByName(args.IfName); err != nil {
			if err != ip.ErrLinkNotFound {
				return err
			}
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
		return logging.Errorf("ip del link failed, error: %v, netns: %s, interface: %s", err, args.Netns, args.IfName)
	}

	return nil
}
