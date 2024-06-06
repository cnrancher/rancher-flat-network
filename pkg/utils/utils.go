package utils

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strconv"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
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

// IsPodEnabledFlatNetwork returns true if the pod enables flatnetwork
func IsPodEnabledFlatNetwork(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Annotations == nil {
		return false
	}
	if pod.Annotations[flv1.AnnotationIP] == "" || pod.Annotations[flv1.AnnotationSubnet] == "" {
		return false
	}
	return true
}

// GetGID returns current go routine ID
func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
