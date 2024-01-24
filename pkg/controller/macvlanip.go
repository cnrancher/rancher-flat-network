package controller

import (
	"time"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
)

func (h *Handler) handleMacvlanIPError(
	onChange func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error),
) func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return func(key string, macvlanip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		var err error
		macvlanip, err = onChange(key, macvlanip)
		if macvlanip == nil {
			return macvlanip, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return macvlanip, err
	}
}

func (h *Handler) onMacvlanIPChanged(s string, macvlanip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if macvlanip == nil || macvlanip.Name == "" || macvlanip.DeletionTimestamp != nil {
		return macvlanip, nil
	}

	h.syncWorkload(macvlanip)

	return macvlanip, nil
}

func (h *Handler) onMacvlanIPRemoved(s string, macvlanip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if macvlanip == nil || macvlanip.Name == "" || macvlanip.DeletionTimestamp != nil {
		return macvlanip, nil
	}

	return macvlanip, nil
}
