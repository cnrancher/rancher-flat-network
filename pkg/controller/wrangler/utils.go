package wrangler

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	discovery "k8s.io/api/discovery/v1"
	discoveryclient "k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func serverSupportDiscoveryV1(
	restCfg *rest.Config,
) (bool, error) {
	c := discoveryclient.NewDiscoveryClientForConfigOrDie(restCfg)
	err := discoveryclient.ServerSupportsVersion(c, discovery.SchemeGroupVersion)
	if err != nil {
		if strings.Contains(err.Error(), "does not support") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check server support %v: %w",
			discovery.GroupName, err)
	}
	return true, nil
}

func serverSupportsIngressV1(k8sClient kubernetes.Interface) bool {
	resources, err := k8sClient.Discovery().ServerResourcesForGroupVersion("networking.k8s.io/v1")
	if err != nil || resources == nil {
		return false
	}
	for _, r := range resources.APIResources {
		if r.Kind == "Ingress" {
			return true
		}
	}
	return false
}

const (
	mutexLocked = 1
)

var (
	ipAllocateMap       = sync.Map{}
	ipAllocateLockMutex = sync.Mutex{}
	isIPAllocatingMutex = sync.Mutex{}
)

// IPAllocateLock locks by subnet name and returns unlock function
func IPAllocateLock(subnet string) func() {
	ipAllocateLockMutex.Lock()
	defer ipAllocateLockMutex.Unlock()

	value, _ := ipAllocateMap.LoadOrStore(subnet, &sync.Mutex{})
	mtx := value.(*sync.Mutex)
	mtx.Lock()
	return func() { mtx.Unlock() }
}

// IsIPAllocating checks whether the subnet is locked
func IsIPAllocating(subnet string) bool {
	isIPAllocatingMutex.Lock()
	defer isIPAllocatingMutex.Unlock()

	o, ok := ipAllocateMap.Load(subnet)
	if !ok {
		return false
	}
	mu, ok := o.(*sync.Mutex)
	if mu == nil || !ok {
		return false
	}

	state := reflect.ValueOf(mu).Elem().FieldByName("state")
	isLocked := (state.Int() & mutexLocked) == mutexLocked
	return isLocked
}
