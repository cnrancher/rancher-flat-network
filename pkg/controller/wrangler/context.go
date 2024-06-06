package wrangler

import (
	"context"
	"fmt"

	flscheme "github.com/cnrancher/flat-network-operator/pkg/generated/clientset/versioned/scheme"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps"
	appsv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch"
	batchv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	fl "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/flatnetwork.cattle.io"
	flv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io"
	networkingv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/rancher/wrangler/v2/pkg/leader"
	"github.com/rancher/wrangler/v2/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

type Context struct {
	RESTConfig *rest.Config

	FlatNetwork flv1.Interface
	Core        corecontroller.Interface
	Apps        appsv1.Interface
	Networking  networkingv1.Interface
	Batch       batchv1.Interface
	Recorder    record.EventRecorder

	leadership *leader.Manager
	starters   []start.Starter
}

func NewContext(
	restCfg *rest.Config,
) (*Context, error) {
	// panic on error
	flatnetwork := fl.NewFactoryFromConfigOrDie(restCfg)
	core := core.NewFactoryFromConfigOrDie(restCfg)
	apps := apps.NewFactoryFromConfigOrDie(restCfg)
	networking := networking.NewFactoryFromConfigOrDie(restCfg)
	batch := batch.NewFactoryFromConfigOrDie(restCfg)

	clientSet, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset: %w", err)
	}

	utilruntime.Must(flscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Warnf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "flat-network-operarto"})

	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	leadership := leader.NewManager("", "flat-network-operator", k8s)

	c := &Context{
		RESTConfig:  restCfg,
		FlatNetwork: flatnetwork.Flatnetwork().V1(),
		Core:        core.Core().V1(),
		Apps:        apps.Apps().V1(),
		Networking:  networking.Networking().V1(),
		Batch:       batch.Batch().V1(),
		Recorder:    recorder,

		leadership: leadership,
	}
	c.starters = append(c.starters, flatnetwork, core, apps, networking, batch)
	return c, nil
}

func (w *Context) OnLeader(f func(ctx context.Context) error) {
	// w.leadership.OnLeader(f)
}

func (c *Context) Start(ctx context.Context, worker int) error {
	// c.leadership.Start(ctx)

	return start.All(ctx, worker, c.starters...)
}
