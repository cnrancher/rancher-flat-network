package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

func (h *Handler) handleStatefulSetError(
	onChange func(string, *appsv1.StatefulSet) (*appsv1.StatefulSet, error),
) func(string, *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	return func(key string, statefulset *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
		var err error
		statefulset, err = onChange(key, statefulset)
		if statefulset == nil {
			return statefulset, err
		}
		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}

		return statefulset, nil
	}
}

func (h *Handler) onStatefulSetChanged(s string, statefulset *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if statefulset == nil || statefulset.Name == "" || statefulset.DeletionTimestamp != nil {
		return statefulset, nil
	}

	h.syncWorkload(statefulset)

	return statefulset, nil
}

func (h *Handler) onStatefulSetRemoved(s string, statefulset *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if statefulset == nil || statefulset.Name == "" || statefulset.DeletionTimestamp != nil {
		return statefulset, nil
	}

	return statefulset, nil
}
