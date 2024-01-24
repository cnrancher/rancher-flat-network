package controller

import (
	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

const (
	eventMacvlanSubnetError = "MacvlanSubnetError"
	eventMacvlanIPError     = "MacvlanIPError"

	messageNoEnoughIP = "No enough ip resouce in subnet: %s"
)

func (h *Handler) recordIPError(
	onChange func(key string, ipConfig *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error),
) func(key string, config *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return onChange
}
