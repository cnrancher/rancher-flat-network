package utils

import (
	"encoding/json"
	"fmt"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/vishvananda/netlink"
)

func LoadCNINetConf(bytes []byte) (*types.NetConf, error) {
	n := &types.NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %w", err)
	}
	return n, nil
}

func SetPromiscOn(iface string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("failed to search iface %q: %w", iface, err)
	}

	if link.Attrs().Promisc != 1 {
		err = netlink.SetPromiscOn(link)
		if err != nil {
			return fmt.Errorf("netlink.SetPromiscOn failed on iface %q: %w", iface, err)
		}
	}
	return nil
}

func MergeIPAMConfig(
	netConf *types.NetConf, flatNetworkIP *flv1.FlatNetworkIP, subnet *flv1.FlatNetworkSubnet,
) ([]byte, error) {
	address := flatNetworkIP.Status.Addr
	routes, gateway := subnet.Spec.Routes, subnet.Spec.Gateway
	enableIPv6 := flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] != ""
	netConf.FlatNetworkConfig.IPAM.Addresses = []types.Address{
		{
			Address: address,
			Gateway: gateway,
		},
	}

	// add 6to4 address for IPv6
	// https://en.wikipedia.org/wiki/6to4
	if enableIPv6 {
		_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet CIDR [%v]: %w",
				subnet.Spec.CIDR, err)
		}
		size, _ := n.Mask.Size()
		if ip6to4CIDR := get6to4CIDR(address, size); len(ip6to4CIDR) != 0 {
			netConf.FlatNetworkConfig.IPAM.Addresses = append(netConf.FlatNetworkConfig.IPAM.Addresses, types.Address{
				Address: ip6to4CIDR,
			})
		}
	}

	if len(routes) != 0 {
		rs := []*cnitypes.Route{}
		for _, v := range routes {
			if v.Iface != "" && v.Iface != "eth1" {
				continue
			}

			_, n, err := net.ParseCIDR(v.Dst)
			if err != nil {
				return nil, fmt.Errorf("failed to parse route destination CIDR [%v]: %w",
					v.Dst, err)
			}
			rs = append(rs, &cnitypes.Route{
				Dst: *n,
				GW:  v.GW,
			})
		}
		netConf.FlatNetworkConfig.IPAM.Routes = rs
	}
	return json.Marshal(netConf)
}

func get6to4CIDR(ip net.IP, size int) net.IP {
	if ip = ip.To4(); ip == nil {
		return nil
	}
	if size <= 0 || size > 32 {
		return nil
	}
	sixtofourSize := 48 - (32 - size)
	tmp := []byte{}
	for _, v := range ip.To4() {
		tmp = append(tmp, v)
	}
	if len(tmp) != 4 {
		return nil
	}
	return net.ParseIP(fmt.Sprintf("2002:%02x:%02x:0:0:0:0:0/%d",
		tmp[0]+tmp[1], tmp[2]+tmp[3], sixtofourSize))
}
