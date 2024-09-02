package upgrade

import (
	"maps"
	"strings"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/workload"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var annotationKeyMap = map[string]string{
	"macvlan.pandaria.cattle.io/ip":          flv1.AnnotationIP,
	"macvlan.pandaria.cattle.io/subnet":      flv1.AnnotationSubnet,
	"macvlan.pandaria.cattle.io/mac":         flv1.AnnotationMac,
	"macvlan.panda.io/ingress":               flv1.AnnotationIngress,
	"macvlan.panda.io/macvlanService":        flv1.AnnotationFlatNetworkService,
	"macvlan.panda.io/ipDelayReuseTimestamp": "",
	"macvlan.panda.io/ipv6to4":               flv1.AnnotationsIPv6to4,
}

const (
	k8sCNINetworksMultiKey     = "k8s.v1.cni.cncf.io/networks"
	rancherFlatNetworkCNIMulti = `[{"name":"rancher-flat-network","interface":"eth1"}]`

	k8sCNINetworksSingleKey     = "k8s.v1.cni.cncf.io/networks"
	rancherFlatNetworkCNISingle = `[{"name":"rancher-flat-network","interface":"eth0"}]`

	macvlanV1NetAttatchDefNameMulti  = `[{"name":"static-macvlan-cni-attach","interface":"eth1"}]`
	macvlanV1NetAttatchDefNameSingle = `[{"name":"static-macvlan-cni-attach","interface":"eth0"}]`
)

func updateAnnotation(o metav1.Object) map[string]string {
	metadata := workload.GetTemplateObjectMeta(o)
	a := metadata.Annotations
	if a == nil {
		return a
	}
	u := maps.Clone(a)
	for k, v := range a {
		if k == k8sCNINetworksMultiKey && v == macvlanV1NetAttatchDefNameMulti {
			u[k] = rancherFlatNetworkCNIMulti
			continue
		}
		if k == k8sCNINetworksSingleKey && v == macvlanV1NetAttatchDefNameSingle {
			u[k] = rancherFlatNetworkCNISingle
			continue
		}
		if !strings.Contains(k, macvlanV1Prefix) {
			continue
		}

		n := annotationKeyMap[k]
		delete(u, k)
		if n != "" {
			u[n] = v
		}
	}
	return u
}
