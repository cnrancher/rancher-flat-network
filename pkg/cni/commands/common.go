package commands

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/cnrancher/rancher-flat-network/pkg/cni/common"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/create"
)

func loadCNINetConf(bytes []byte) (*types.NetConf, error) {
	n := &types.NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %w", err)
	}
	return n, nil
}

func mergeIPAMConfig(
	netConf *types.NetConf, flatNetworkIP *flv1.FlatNetworkIP, subnet *flv1.FlatNetworkSubnet,
) ([]byte, error) {
	address := flatNetworkIP.Status.Addr
	_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet CIDR: %w", err)
	}
	ones, _ := n.Mask.Size()
	routes, gateway := subnet.Spec.Routes, subnet.Spec.Gateway
	enable6to4 := flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] != ""
	netConf.IPAM.Addresses = []types.Address{
		{
			Address: fmt.Sprintf("%v/%v", address.String(), ones),
			Gateway: gateway,
		},
	}

	// add 6to4 address for IPv6
	// https://en.wikipedia.org/wiki/6to4
	if enable6to4 {
		_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet CIDR [%v]: %w",
				subnet.Spec.CIDR, err)
		}
		size, _ := n.Mask.Size()
		if ip6to4CIDR := get6to4CIDR(address, size); len(ip6to4CIDR) != 0 {
			netConf.IPAM.Addresses = append(netConf.IPAM.Addresses, types.Address{
				Address: ip6to4CIDR,
			})
		}
	}

	if len(routes) != 0 {
		rs := []*cnitypes.Route{}
		for _, v := range routes {
			if v.Dev != "" && v.Dev == common.PodIfaceEth0 {
				continue
			}

			_, n, err := net.ParseCIDR(v.Dst)
			if err != nil {
				return nil, fmt.Errorf("failed to parse route destination CIDR [%v]: %w",
					v.Dst, err)
			}
			rs = append(rs, &cnitypes.Route{
				Dst: *n,
				GW:  v.Via,
			})
		}
		netConf.IPAM.Routes = rs
	}
	logrus.Debugf("merged IPAM addresses: %v", utils.Print(netConf.IPAM))
	return json.Marshal(netConf)
}

func get6to4CIDR(ip net.IP, size int) string {
	if ip = ip.To4(); ip == nil {
		return ""
	}
	if size <= 0 || size > 32 {
		return ""
	}
	sixtofourSize := 48 - (32 - size)
	tmp := []byte{}
	for _, v := range ip.To4() {
		tmp = append(tmp, v)
	}
	if len(tmp) != 4 {
		return ""
	}
	return fmt.Sprintf("2002:%02x:%02x:0:0:0:0:0/%d",
		tmp[0]+tmp[1], tmp[2]+tmp[3], sixtofourSize)
}

// parsePrevResult parses a prevResult in a NetConf structure and sets
// the NetConf's PrevResult member to the parsed Result object.
func parsePrevResult(conf *types.NetConf) error {
	if conf.RawPrevResult == nil {
		return nil
	}

	// Prior to 1.0.0, Result types may not marshal a CNIVersion. Since the
	// result version must match the config version, if the Result's version
	// is empty, inject the config version.
	if ver, ok := conf.RawPrevResult["CNIVersion"]; !ok || ver == "" {
		conf.RawPrevResult["CNIVersion"] = conf.CNIVersion
	}

	resultBytes, err := json.Marshal(conf.RawPrevResult)
	if err != nil {
		return fmt.Errorf("could not serialize prevResult: %w", err)
	}

	conf.RawPrevResult = nil
	conf.PrevResult, err = create.Create(conf.CNIVersion, resultBytes)
	if err != nil {
		return fmt.Errorf("could not parse prevResult: %w", err)
	}

	return nil
}
