package wrangler

import (
	"fmt"
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

type ipAllocatingMutex struct {
	m        sync.Mutex
	isLocked bool
}

func (m *ipAllocatingMutex) lock() {
	m.isLocked = true
	m.m.Lock()
}

func (m *ipAllocatingMutex) unlock() {
	m.m.Unlock()
	m.isLocked = false
}

var (
	ipAllocateMap       = sync.Map{}
	ipAllocateLockMutex = sync.Mutex{}
	isIPAllocatingMutex = sync.Mutex{}
)

// IPAllocateLock locks by subnet name and returns unlock function
func IPAllocateLock(subnet string) func() {
	ipAllocateLockMutex.Lock()
	defer ipAllocateLockMutex.Unlock()

	value, _ := ipAllocateMap.LoadOrStore(subnet, &ipAllocatingMutex{})
	mtx := value.(*ipAllocatingMutex)
	mtx.lock()
	return func() { mtx.unlock() }
}

// IsIPAllocating checks whether the subnet is locked
func IsIPAllocating(subnet string) bool {
	isIPAllocatingMutex.Lock()
	defer isIPAllocatingMutex.Unlock()

	o, ok := ipAllocateMap.Load(subnet)
	if !ok {
		return false
	}
	mu, ok := o.(*ipAllocatingMutex)
	if mu == nil || !ok {
		return false
	}

	return mu.isLocked
}
