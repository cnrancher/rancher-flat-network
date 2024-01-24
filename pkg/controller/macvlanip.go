package controller

import (
	"fmt"
	"strings"
	"time"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	macvlanIPInitPhase    = ""
	macvlanIPPendingPhase = "Pending"
	macvlanIPActivePhase  = "Active"
	macvlanIPFailedPhase  = "Failed"
)

func (h *Handler) handleMacvlanIPError(
	onChange func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error),
) func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return func(key string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		var message string
		var err error
		ip, err = onChange(key, ip)
		if ip == nil {
			return ip, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			message = err.Error()
		}
		if ip.Name == "" {
			return ip, err
		}

		if ip.Status.FailureMessage == message {
			// Avoid trigger the rate limit.
			if message != "" {
				time.Sleep(time.Second * 5)
			}
			return ip, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			ip, err := h.macvlanIPs.Get(ip.Namespace, ip.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			ip = ip.DeepCopy()
			if message != "" {
				// can assume an update is failing
				ip.Status.Phase = macvlanIPFailedPhase
			}
			ip.Status.FailureMessage = message

			_, err = h.macvlanIPs.UpdateStatus(ip)
			return err
		})
		if err != nil {
			logrus.Errorf("Error recording macvlan IP config [%s] failure message: %v", ip.Name, err)
			return ip, err
		}
		return ip, nil
	}
}

func (h *Handler) onMacvlanIPRemoved(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" {
		return ip, nil
	}

	return ip, nil
}

func (h *Handler) onMacvlanIPChanged(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" || ip.DeletionTimestamp != nil {
		return ip, nil
	}

	switch ip.Status.Phase {
	case macvlanIPActivePhase:
		return h.updateMacvlanIP(ip)
	default:
		return h.createMacvlanIP(ip)
	}
}

func (h *Handler) createMacvlanIP(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Update macvlan IP status to active
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		ip, err := h.macvlanIPs.Get(ip.Namespace, ip.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		ip = ip.DeepCopy()
		// can assume an update is failing
		ip.Status.Phase = macvlanIPActivePhase

		_, err = h.macvlanIPs.UpdateStatus(ip)
		return err
	})
	if err != nil {
		logrus.Errorf("Error recording macvlan IP config [%s] failure message: %v", ip.Name, err)
	}
	logrus.Infof("Create macvlan ip Name [%v] Subnet [%v] CIDR [%v]",
		ip.Name, ip.Spec.Subnet, ip.Spec.CIDR)

	return ip, nil
}

func (h *Handler) updateMacvlanIP(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Re-add missing records in cache
	// If a Pod was deleted with a duplicate IP in badpods purging process,
	// it may cause the IP record to be lost in the cache
	key := fmt.Sprintf("%s:%s", strings.SplitN(ip.Spec.CIDR, "/", 2)[0], ip.Spec.Subnet)
	if _, ok := h.inUsedIPs.Load(key); !ok {
		// use api client to get the latest resource version
		// pod, _ := c.kubeClientset.CoreV1().Pods(ip.Namespace).Get(context.TODO(), ip.Name, metav1.GetOptions{})
		pod, _ := h.pods.Get(ip.Namespace, ip.Name, metav1.GetOptions{})
		if pod != nil && pod.DeletionTimestamp == nil && pod.Name != "" {
			owner := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
			h.inUsedIPs.Store(key, owner)
			logrus.Infof("updateMacvlanIP: re-add key %s value %s to the syncmap", key, owner)
		}
	}

	// TODO:
	// if oldip.ResourceVersion != ip.ResourceVersion && oldip.Spec.CIDR != ip.Spec.CIDR {
	// 	// remove the old record from cache
	// 	// to address the statfuleset pod case
	// 	oldkey := fmt.Sprintf("%s:%s", strings.SplitN(oldip.Spec.CIDR, "/", 2)[0], oldip.Spec.Subnet)
	// 	c.inUsedIPs.Delete(oldkey)
	// 	log.Infof("onMacvlanIPUpdate: remove key %s from syncmap as macvlanip record %s was updated", oldkey, ip.Name)
	// }

	// IP delayed release, only in auto mode
	if ip.Labels[macvlanv1.LabelMacvlanIPType] != "auto" {
		return ip, nil
	}

	// subnetName := ip.Labels["subnet"]
	// subnet, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnetName, metav1.GetOptions{})
	// if err != nil {
	// 	logrus.Errorf("onMacvlanIPUpdate: %s subnet %s not exist", ip.Name, subnetName)
	// 	subnet = &macvlanv1.MacvlanSubnet{}
	// }
	// if ip.DeletionTimestamp != nil {
	// 	if ip.Annotations[macvlanv1.AnnotationIPDelayReuse] == "" {
	// 		c.updateDelayReuseTimestamp(ip, subnet.Spec.IPDelayReuse)
	// 		return
	// 	}
	// 	c.calcNeedRemoveDelayReuseMacvlanIP(ip)
	// 	return
	// }

	// if subnet.Spec.IPDelayReuse != 0 && !slices.Contains(ip.ObjectMeta.Finalizers, macvlanv1.FinalizerIPDelayReuse) {
	// 	c.updateIPDelayReuseFinalizer(ip)
	// }

	return ip, nil
}
