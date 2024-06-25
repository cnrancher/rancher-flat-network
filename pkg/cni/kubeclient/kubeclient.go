package kubeclient

import (
	"context"
	"fmt"
	"strings"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	clientset "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/clientset/versioned"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	subnetNamespace   = "kube-system"
	defaultKubeConfig = "/etc/cni/net.d/multus.d/multus.kubeconfig"
)

type defaultKubeClient struct {
	client           kubernetes.Interface
	macvlanclientset clientset.Interface
}

// defaultKubeClient implements KubeClient
var _ KubeClient = &defaultKubeClient{}

type KubeClient interface {
	GetPod(context.Context, string, string) (*v1.Pod, error)

	GetFlatNetworkIP(context.Context, string, string) (*flv1.FlatNetworkIP, error)
	UpdateFlatNetworkIP(context.Context, string, *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error)
	GetFlatNetworkSubnet(context.Context, string) (*flv1.FlatNetworkSubnet, error)
}

func (d *defaultKubeClient) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	return d.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (d *defaultKubeClient) GetFlatNetworkIP(ctx context.Context, namespace, name string) (*flv1.FlatNetworkIP, error) {
	return d.macvlanclientset.FlatnetworkV1().FlatNetworkIPs(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (d *defaultKubeClient) UpdateFlatNetworkIP(ctx context.Context, namespace string, macvlanip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	return d.macvlanclientset.FlatnetworkV1().FlatNetworkIPs(namespace).Update(ctx, macvlanip, metav1.UpdateOptions{})
}

func (d *defaultKubeClient) GetFlatNetworkSubnet(ctx context.Context, name string) (*flv1.FlatNetworkSubnet, error) {
	return d.macvlanclientset.FlatnetworkV1().FlatNetworkSubnets(subnetNamespace).Get(ctx, name, metav1.GetOptions{})
}

func detectKubeConfig(cniPath string) string {
	if strings.Contains(cniPath, "k3s") {
		return fmt.Sprintf("/var/lib/rancher/k3s/agent%s", defaultKubeConfig)
	}
	return defaultKubeConfig
}

func GetK8sArgs(args *skel.CmdArgs) (*types.K8sArgs, error) {
	k8sArgs := &types.K8sArgs{}

	err := cnitypes.LoadArgs(args.Args, k8sArgs)
	if err != nil {
		return nil, err
	}

	return k8sArgs, nil
}

func GetK8sClient(cniPath string) (KubeClient, error) {
	var err error
	var config *rest.Config

	// uses the current context in kubeconfig
	config, err = clientcmd.BuildConfigFromFlags("", detectKubeConfig(cniPath))
	if err != nil {
		return nil, fmt.Errorf("GetK8sClient: failed to get context for the kubeconfig %v, refer Multus README.md for the usage guide: %v", defaultKubeConfig, err)
	}

	config.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	// config.ContentType = "application/vnd.kubernetes.protobuf"
	config.ContentType = "application/json"

	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	macvlanclientset, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &defaultKubeClient{client: client, macvlanclientset: macvlanclientset}, nil
}
