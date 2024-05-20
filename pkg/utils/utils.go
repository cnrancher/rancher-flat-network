package utils

import (
	"encoding/json"
	"net"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corev1 "k8s.io/api/core/v1"
)

func PrintObject(a any) string {
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}

type valueTypes interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 |
		~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64 | ~string | ~bool |
		[]string
}

// Pointer gets the pointer of the variable.
func Pointer[T valueTypes](i T) *T {
	return &i
}

// A safe function to get the value from the pointer.
func Value[T valueTypes](p *T) T {
	if p == nil {
		return *new(T)
	}
	return *p
}

// IsMacvlanPod returns true if the pod enables macvlan
func IsMacvlanPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Annotations == nil {
		return false
	}
	if pod.Annotations[macvlanv1.AnnotationIP] == "" || pod.Annotations[macvlanv1.AnnotationSubnet] == "" {
		return false
	}
	return true
}

func IsSingleIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func IsMultipleIP(ip string) bool {
	if !strings.Contains(ip, "-") {
		return false
	}
	ips := strings.Split(strings.TrimSpace(ip), "-")

	if len(ips) < 2 {
		return false
	}

	for _, v := range ips {
		if net.ParseIP(v) == nil {
			return false
		}
	}
	return true
}
