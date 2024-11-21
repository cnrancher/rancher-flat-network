package commands

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/cnrancher/rancher-flat-network/pkg/cni/common"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/types"
	"github.com/stretchr/testify/assert"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
)

func Test_MergeIPAMConfig(t *testing.T) {
	assert := assert.New(t)
	s := `{
		"cniVersion": "1.0.0",
		"type": "rancher-flat-network-cni",
		"master": "",
		"ipam": {
		  "type": "static-ipam"
		}
	  }`

	n, err := loadCNINetConf([]byte(s))
	if err != nil {
		t.Error(err)
	}

	flip := &flv1.FlatNetworkIP{
		Spec: flv1.IPSpec{},
		Status: flv1.IPStatus{
			Phase:          "",
			FailureMessage: "",
			Addr:           net.ParseIP("192.168.1.2"),
		},
	}
	flsubnet := &flv1.FlatNetworkSubnet{
		Spec: flv1.SubnetSpec{
			FlatMode: "macvlan",
			Master:   "eth0",
			VLAN:     0,
			CIDR:     "192.168.1.0/24",
			Mode:     "bridge",
			Gateway:  net.ParseIP("192.168.1.1"),
			Ranges:   nil,
			Routes: []flv1.Route{
				{
					Dev: common.PodIfaceEth1,
					Dst: "192.168.2.0/24",
					Src: net.ParseIP(""),
					Via: net.ParseIP("192.168.2.1"),
				},
				{
					Dev: common.PodIfaceEth0,
					Dst: "10.44.1.0/24",
				},
			},
			RouteSettings: flv1.RouteSettings{
				AddClusterCIDR: false,
				AddServiceCIDR: false,
				AddNodeCIDR:    false,
				AddPodIPToHost: false,
			},
		},
	}

	// Merge IPAM Config when using single nic mode (eth0)
	c, err := mergeIPAMConfig(common.PodIfaceEth0, n, flip, flsubnet)
	if err != nil {
		t.Error(err)
		return
	}
	result := types.NetConf{}
	json.Unmarshal(c, &result)
	assert.Equal(1, len(result.IPAM.Addresses))
	assert.Equal("192.168.1.2/24", result.IPAM.Addresses[0].Address)
	assert.Equal("192.168.1.1", result.IPAM.Addresses[0].Gateway.String())
	assert.Equal(0, len(result.IPAM.Routes)) // single nic mode should not have routes

	// Merge IPAM Config when using multi-nic mode (eth1)
	c, err = mergeIPAMConfig(common.PodIfaceEth1, n, flip, flsubnet)
	if err != nil {
		t.Error(err)
		return
	}
	json.Unmarshal(c, &result)
	assert.Equal(1, len(result.IPAM.Addresses))
	assert.Equal("192.168.1.2/24", result.IPAM.Addresses[0].Address)
	assert.Equal("192.168.1.1", result.IPAM.Addresses[0].Gateway.String())
	assert.Equal(1, len(result.IPAM.Routes)) // multi-nic mode should have routes on eth1
	assert.Equal("192.168.2.0/24", result.IPAM.Routes[0].Dst.String())
	assert.Equal("192.168.2.1", result.IPAM.Routes[0].GW.String())

	fmt.Println(string(c))
}
