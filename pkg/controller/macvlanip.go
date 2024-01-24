package controller

import (
	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

func (h *Handler) onMacvlanIPUpdate(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return ip, nil
}

// func (h *Handler) syncMacvlanIP(key string, obj *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
// 	if obj == nil {
// 		return nil, nil
// 	}
// 	logrus.Infof("XXXXX syncMacvlanIP, resourveVersion: %v", obj.ResourceVersion)

// 	origStatus := obj.Status.DeepCopy()
// 	obj = obj.DeepCopy()
// 	newStatus, err := a.handler(obj, obj.Status)
// 	if err != nil {
// 		// Revert to old status on error
// 		newStatus = *origStatus.DeepCopy()
// 	}

// 	return ip, nil
// }
