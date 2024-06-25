package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func Check(args *skel.CmdArgs) error {
	logrus.Debugf("cmdCheck args: %v", utils.Print(args))

	n, err := loadCNINetConf(args.StdinData)
	if err != nil {
		return err
	}
	isLayer3 := n.FlatNetworkConfig.IPAM.Type != ""

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	if isLayer3 {
		// run the IPAM plugin and get back the config to apply
		err = ipam.ExecCheck(n.FlatNetworkConfig.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	// Parse previous result.
	if n.RawPrevResult == nil {
		return fmt.Errorf("required prevResult missing")
	}

	if err := parsePrevResult(n); err != nil {
		return err
	}

	result, err := types100.NewResultFromResult(n.PrevResult)
	if err != nil {
		return err
	}

	var contMap types100.Interface
	// Find interfaces for names whe know, macvlan device name inside container
	for _, intf := range result.Interfaces {
		if args.IfName == intf.Name {
			if args.Netns == intf.Sandbox {
				contMap = *intf
				continue
			}
		}
	}

	// The namespace must be the same as what was configured
	if args.Netns != contMap.Sandbox {
		return fmt.Errorf("sandbox in prevResult %s doesn't match configured netns: %s",
			contMap.Sandbox, args.Netns)
	}

	// if n.LinkContNs {
	// 	err = netns.Do(func(_ ns.NetNS) error {
	// 		_, err = netlink.LinkByName(n.Master)
	// 		return err
	// 	})
	// } else {
	// 	_, err = netlink.LinkByName(n.Master)
	// }
	_, err = netlink.LinkByName(n.FlatNetworkConfig.Master)
	if err != nil {
		return fmt.Errorf("failed to lookup master %q: %v",
			n.FlatNetworkConfig.Master, err)
	}

	// Check prevResults for ips, routes and dns against values found in the container
	if err := netns.Do(func(_ ns.NetNS) error {
		// Check interface against values found in the container
		err := validateCniContainerInterface(contMap, n.FlatNetworkConfig.Mode)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedInterfaceIPs(args.IfName, result.IPs)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedRoute(result.Routes)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
