package namespace

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/controller/wrangler"
	corecontroller "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/controllers/core/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	k8scnicncfiov1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ndClientSet "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
)

const (
	handlerName = "rancher-flat-network-namespace"

	k8sCNINetworksKey = "k8s.v1.cni.cncf.io/networks"
	netAttatchDefName = "rancher-flat-network"

	arpPolicyEnv     = "FLAT_NETWORK_CNI_ARP_POLICY"
	proxyARPEnv      = "FLAT_CNI_PROXY_ARP"
	defaultARPPolicy = "arping"
)

type handler struct {
	namespaceClient corecontroller.NamespaceClient
	ndClientSet     *ndClientSet.Clientset

	nsEnqueueAfter func(string, string, time.Duration)
	nsEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		namespaceClient: wctx.Core.Namespace(),
		ndClientSet:     wctx.NDClientSet,

		nsEnqueueAfter: wctx.Core.Endpoints().EnqueueAfter,
		nsEnqueue:      wctx.Core.Endpoints().Enqueue,
	}
	wctx.Core.Namespace().OnChange(ctx, handlerName, h.syncNamespace)
}

func (h *handler) syncNamespace(
	_ string, ns *corev1.Namespace,
) (*corev1.Namespace, error) {
	expectedNetworkAttachDef := newNetworkAttachmentDefinition(netAttatchDefName, ns)
	existNetworkAttachDef, err := h.ndClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(ns.Name).
		Get(context.TODO(), netAttatchDefName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = h.ndClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(ns.Name).Create(
				context.TODO(), expectedNetworkAttachDef, metav1.CreateOptions{})
			if err != nil {
				logrus.WithFields(fieldsNS(ns)).
					Errorf("failed to create netAttachmentDef [%v/%v] config: %v",
						ns.Name, netAttatchDefName, err)
				return ns, err
			}
			logrus.WithFields(fieldsNS(ns)).
				Infof("create netAttachmentDef [%v] for namespace [%v]",
					netAttatchDefName, ns.Name)
			return ns, nil
		}
		logrus.WithFields(fieldsNS(ns)).
			Errorf("failed to get netAttachmentDef [%v/%v]: %v",
				ns.Name, netAttatchDefName, err)
		return ns, err
	}

	if expectedNetworkAttachDef.Spec.Config == existNetworkAttachDef.Spec.Config {
		logrus.WithFields(fieldsNS(ns)).
			Debugf("netAttachmentDef [%v/%v] already exists, skip",
				ns.Name, netAttatchDefName)
		return ns, err
	}

	_, err = h.ndClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(ns.Name).Update(
		context.TODO(), expectedNetworkAttachDef, metav1.UpdateOptions{})
	if err != nil {
		logrus.WithFields(fieldsNS(ns)).
			Errorf("failed to update netAttachmentDef [%v/%v] config: %v",
				ns.Name, netAttatchDefName, err)
		return ns, err
	}
	logrus.WithFields(fieldsNS(ns)).
		Infof("update netAttachmentDef [%v/%v] config",
			ns.Name, netAttatchDefName)
	return ns, nil
}

func newNetworkAttachmentDefinition(
	name string, ns *corev1.Namespace,
) *k8scnicncfiov1.NetworkAttachmentDefinition {
	return &k8scnicncfiov1.NetworkAttachmentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkAttachmentDefinition",
			APIVersion: "k8s.cni.cncf.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns.Name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       ns.Name,
					UID:        ns.UID,
					Controller: utils.Ptr(true),
				},
			},
		},
		Spec: k8scnicncfiov1.NetworkAttachmentDefinitionSpec{
			Config: getNetAttachDefConfig(),
		},
	}
}

func getNetAttachDefConfig() string {
	netAttachDefConfig := `{
		"cniVersion": "0.3.1",
		"type": "static-macvlan-cni",
		"master": "",
		"runtimeConfig": {
			"arpPolicy": "` + getARPPolicy() + `",
			"proxyARP": ` + getProxyARP() + `
		},
		"ipam": {
			"type": "static-ipam"
		}
	}`
	return netAttachDefConfig
}

func getARPPolicy() string {
	arpPolicy := os.Getenv(arpPolicyEnv)
	if arpPolicy != "" {
		return arpPolicy
	}
	return defaultARPPolicy
}

func getProxyARP() string {
	flag, _ := strconv.ParseBool(os.Getenv(proxyARPEnv))
	return strconv.FormatBool(flag)
}

func fieldsNS(ns *corev1.Namespace) logrus.Fields {
	if ns == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GID(),
		"NS":  ns.Name,
	}
}
