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

// syncService auto create/delete svc for macvlan pod
func (h *Handler) syncService(pod *corev1.Pod) error {
	// do nothing if pod has no owner
	if pod.OwnerReferences == nil || len(pod.OwnerReferences) == 0 {
		return nil
	}

	ownerName, ownerKind, ownerUID, err := h.findOwnerWorkload(pod)
	if err != nil {
		return err
	}
	logrus.Debugf("syncService: %s is own by workload %s/%s", pod.Name, pod.Namespace, ownerName)

	var svc *corev1.Service
	retryErr := retry.OnError(svcGetRetryBackoff, apierrors.IsNotFound, func() error {
		var stepErr error
		// svc, stepErr = h.kubeClientset.CoreV1().Services(pod.Namespace).Get(context.TODO(), ownerName, metav1.GetOptions{})
		svc, stepErr = h.services.Get(pod.Namespace, ownerName, metav1.GetOptions{})
		logrus.Debugf("syncService: get svc err %v", stepErr)
		return stepErr
	})
	if retryErr != nil {
		return retryErr
	}

	logrus.Debugf("syncService: get origin svc %s/%s", svc.Namespace, svc.Name)
	macvlanService := makeService(ownerUID, ownerKind, svc)
	logrus.Debugf("syncService: to create/update new macvlan svc %s/%s", macvlanService.Namespace, macvlanService.Name)
	err = h.updateService(macvlanService)
	if err != nil {
		return err
	}
	return nil
}

func (c *Handler) updateService(svc *corev1.Service) error {
	// get, err := c.kubeClientset.CoreV1().Services(svc.Namespace).Get(context.TODO(), svc.Name, metav1.GetOptions{})
	get, err := c.services.Get(svc.Namespace, svc.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// _, err = c.kubeClientset.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
			_, err = c.services.Create(svc)
			return err
		}
		return err
	}
	svc.ResourceVersion = get.ResourceVersion
	// _, err = c.kubeClientset.CoreV1().Services(svc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})
	_, err = c.services.Update(svc)
	return err
}

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
		}
	}
	return "", "", "", fmt.Errorf("%s owner workload not found", pod.Name)
}
