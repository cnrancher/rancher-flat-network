package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
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

const (
	// Operator auto-created flat-network service name suffix
	FlatNetworkServiceNameSuffix = "-flat-network"

	NetAttatchDefName = "rancher-flat-network"

	V1NetAttachDefName = "static-macvlan-cni-attach"

	clusterIPNone = "None"
)

func IsMacvlanV1Service(svc *corev1.Service) bool {
	if !strings.HasSuffix(svc.Name, "-macvlan") {
		return false
	}
	if svc.Spec.Type != "ClusterIP" {
		return false
	}
	if len(svc.Spec.ClusterIPs) != 0 {
		if svc.Spec.ClusterIPs[0] != clusterIPNone {
			return false
		}
	} else if svc.Spec.ClusterIP != clusterIPNone {
		return false
	}
	if svc.Annotations == nil {
		return false
	}
	if svc.Annotations[nettypes.NetworkAttachmentAnnot] != V1NetAttachDefName {
		return false
	}
	return true
}

// Check if this service is a flat-network service.
//
// Specification:
// A Flat-Network Service is a ClusterIP typed headless service, name ends with
// '-flat-network' suffix.
// And should have 'k8s.v1.cni.cncf.io/networks: rancher-flat-network' annotation.
func IsFlatNetworkService(svc *corev1.Service) bool {
	if !strings.HasSuffix(svc.Name, FlatNetworkServiceNameSuffix) {
		return false
	}
	if svc.Spec.Type != "ClusterIP" {
		return false
	}
	if len(svc.Spec.ClusterIPs) != 0 {
		if svc.Spec.ClusterIPs[0] != clusterIPNone {
			return false
		}
	} else if svc.Spec.ClusterIP != clusterIPNone {
		return false
	}
	if svc.Annotations == nil {
		return false
	}
	if svc.Annotations[nettypes.NetworkAttachmentAnnot] != NetAttatchDefName {
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

func PromptUser(ctx context.Context, text string, autoYes bool) error {
	var s string
	fmt.Printf("%v [Y/n] ", text)
	if autoYes {
		fmt.Println("y")
	} else {
		if _, err := Scanf(ctx, "%s", &s); err != nil {
			return err
		}
		if len(s) == 0 {
			return nil
		}
		if s[0] != 'y' && s[0] != 'Y' {
			return fmt.Errorf("process canceled by user")
		}
	}
	return nil
}

func Scanf(ctx context.Context, format string, a ...any) (int, error) {
	nCh := make(chan int)
	go func() {
		n, _ := fmt.Scanf(format, a...)
		nCh <- n
	}()
	select {
	case n := <-nCh:
		return n, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func CheckFileExistsPrompt(
	ctx context.Context, name string, autoYes bool,
) error {
	_, err := os.Stat(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	var s string
	fmt.Printf("File %q already exists! Overwrite? [Y/n] ", name)
	if autoYes {
		fmt.Println("y")
		return nil
	}
	if _, err := Scanf(ctx, "%s", &s); err != nil {
		return err
	}
	if len(s) != 0 && s[0] != 'y' && s[0] != 'Y' {
		return fmt.Errorf("file %q already exists", name)
	}
	return nil
}

func SetupLogrus(debug bool) {
	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        false,
		TimestampFormat: time.DateTime,
	})
	logrus.SetOutput(io.Discard)
	logrus.AddHook(&writer.Hook{
		// Send logs with level higher than warning to stderr.
		Writer: os.Stderr,
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
		},
	})
	logrus.AddHook(&writer.Hook{
		// Send info, debug and trace logs to stdout.
		Writer: os.Stdout,
		LogLevels: []logrus.Level{
			logrus.TraceLevel,
			logrus.InfoLevel,
			logrus.DebugLevel,
		},
	})
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debug output enabled")
	}
}
