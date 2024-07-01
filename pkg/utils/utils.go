package utils

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strconv"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

var hostname string

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		logrus.Errorf("failed to get hostname: %v", err)
	}
}

func Print(a any) string {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		logrus.Warnf("utils.Print: failed to json marshal (%T): %v", a, err)
	}
	return string(b)
}

type valueTypes interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 |
		~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64 | ~string | ~bool |
		[]string
}

// Ptr gets the pointer of the variable.
func Ptr[T valueTypes](i T) *T {
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

// GID returns current go routine ID
func GID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// Hostname returns current hostname.
func Hostname() string {
	return hostname
}
