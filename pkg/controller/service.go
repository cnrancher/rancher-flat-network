package controller

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

var svcGetRetryBackoff = wait.Backoff{
	Steps:    6,
	Duration: 100 * time.Millisecond,
	Factor:   1,
	Cap:      1 * time.Second,
}

// syncService will auto create/delete service for macvlan pod.
func (h *Handler) syncService(pod *corev1.Pod) error {
	// Do nothing if the pod do not have owner reference.
	if pod.OwnerReferences == nil || len(pod.OwnerReferences) == 0 {
		return nil
	}

	ownerName, ownerKind, ownerUID, err := h.findOwnerWorkload(pod)
	if err != nil {
		return err
	}
	logrus.Debugf("syncService: %s is own by workload %s/%s", pod.Name, pod.Namespace, ownerName)

	var svc *corev1.Service
	err = retry.OnError(svcGetRetryBackoff, apierrors.IsNotFound, func() error {
		svc, err = h.services.Get(pod.Namespace, ownerName, metav1.GetOptions{})
		logrus.Debugf("syncService: get svc err %v", err)
		return err
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("syncService: Skip to create macvlan service for pod [%v/%v]: %v",
				pod.Namespace, pod.Name, err)
			return nil
		}
		return err
	}

	// TODO: Move makeService into separate package.
	logrus.Debugf("syncService: get origin svc %s/%s", svc.Namespace, svc.Name)
	macvlanService := makeService(ownerUID, ownerKind, svc)
	logrus.Debugf("syncService: to create/update new macvlan svc %s/%s", macvlanService.Namespace, macvlanService.Name)
	if err := h.updateService(macvlanService); err != nil {
		return err
	}
	return nil
}

func (c *Handler) updateService(svc *corev1.Service) error {
	get, err := c.services.Get(svc.Namespace, svc.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if _, err = c.services.Create(svc); err != nil {
				logrus.Warnf("Failed to create service %q: %v", svc.Name, err)
				return err
			}
			logrus.Infof("Created service %q", svc.Name)
		}
		return err
	}

	// TODO: Use retry
	// TODO: Do not modify the resourceVersion here directly
	svc.ResourceVersion = get.ResourceVersion
	_, err = c.services.Update(svc)
	return err
}

// TODO: Move the makeService into separate project
func makeService(uid types.UID, kind string, svc *corev1.Service) *corev1.Service {
	ports := []corev1.ServicePort{}

	for _, v := range svc.Spec.Ports {
		port := v.DeepCopy()
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			port.Port = port.Port + 1
			port.TargetPort = intstr.FromInt(port.TargetPort.IntValue() + 1)
		}
		ports = append(ports, *port)
	}

	specCopy := svc.Spec.DeepCopy()
	specCopy.ClusterIP = corev1.ClusterIPNone
	specCopy.ClusterIPs = nil
	specCopy.Ports = ports

	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-macvlan", svc.Name),
			Namespace:       svc.Namespace,
			OwnerReferences: svc.OwnerReferences,
			Annotations: map[string]string{
				k8sCNINetworksKey: netAttatchDefName,
			},
		},
		Spec: *specCopy,
	}

	return s
}

// TODO: Move the findOwnerWorkload into utils package
func (h *Handler) findOwnerWorkload(pod *corev1.Pod) (string, string, types.UID, error) {
	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "DaemonSet":
			d, err := h.daemonsets.Get(pod.Namespace, owner.Name, metav1.GetOptions{})
			if err != nil {
				return "", "", "", err
			}
			return d.GetName(), d.Kind, d.UID, nil
		case "StatefulSet":
			s, err := h.statefulsets.Get(pod.Namespace, owner.Name, metav1.GetOptions{})
			if err != nil {
				return "", "", "", err
			}
			return s.GetName(), s.Kind, s.UID, nil
		case "Deployment":
			d, err := h.deployments.Get(pod.Namespace, owner.Name, metav1.GetOptions{})
			if err != nil {
				return "", "", "", err
			}
			return d.GetName(), d.Kind, d.UID, nil
		case "ReplicaSet":
			rs, err := h.replicasets.Get(pod.Namespace, owner.Name, metav1.GetOptions{})
			if err != nil {
				return "", "", "", err
			}
			if rs.OwnerReferences == nil || len(rs.OwnerReferences) < 1 {
				return "", "", "", fmt.Errorf("pod owner is empty")
			}
			if rs.OwnerReferences[0].Kind != "Deployment" {
				return "", "", "", fmt.Errorf("pod owner is invalid kind: %s", rs.OwnerReferences[0].Kind)
			}
			d, err := h.deployments.Get(pod.Namespace, rs.OwnerReferences[0].Name, metav1.GetOptions{})
			if err != nil {
				return "", "", "", err
			}
			return d.GetName(), d.Kind, d.UID, nil
		default:
			logrus.Warnf("Failed to get owner name of resource %q", owner.Name)
		}
	}
	return "", "", "", fmt.Errorf("%s owner workload not found", pod.Name)
}
