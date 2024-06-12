package wrangler

import (
	"context"
	"sync"

	flscheme "github.com/cnrancher/flat-network-operator/pkg/generated/clientset/versioned/scheme"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps"
	appsv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch"
	batchv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	fl "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/flatnetwork.pandaria.io"
	flv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io"
	networkingv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/v2/pkg/leader"
	"github.com/rancher/wrangler/v2/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

type Context struct {
	RESTConfig        *rest.Config
	ControllerFactory controller.SharedControllerFactory

	FlatNetwork flv1.Interface
	Core        corecontroller.Interface
	Apps        appsv1.Interface
	Networking  networkingv1.Interface
	Batch       batchv1.Interface
	Recorder    record.EventRecorder

	leadership     *leader.Manager
	starters       []start.Starter
	controllerLock sync.Mutex
}

func NewContextOrDie(
	restCfg *rest.Config,
) *Context {
	// panic on error
	flatnetwork := fl.NewFactoryFromConfigOrDie(restCfg)
	core := core.NewFactoryFromConfigOrDie(restCfg)
	apps := apps.NewFactoryFromConfigOrDie(restCfg)
	networking := networking.NewFactoryFromConfigOrDie(restCfg)
	batch := batch.NewFactoryFromConfigOrDie(restCfg)

	clientSet, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logrus.Fatalf("failed to build clientset: %v", err)
	}

	controllerFactory, err := controller.NewSharedControllerFactoryFromConfig(restCfg, runtime.NewScheme())
	if err != nil {
		logrus.Fatalf("failed to build shared controller factory: %v", err)
	}

	utilruntime.Must(flscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Warnf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "flat-network-operator"})

	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logrus.Fatalf("kubernetes.NewForConfig: %v", err)
	}
	leadership := leader.NewManager("kube-system", "flat-network-operator", k8s)

	c := &Context{
		RESTConfig:        restCfg,
		ControllerFactory: controllerFactory,

		FlatNetwork: flatnetwork.Flatnetwork().V1(),
		Core:        core.Core().V1(),
		Apps:        apps.Apps().V1(),
		Networking:  networking.Networking().V1(),
		Batch:       batch.Batch().V1(),
		Recorder:    recorder,

		leadership: leadership,
	}
	c.starters = append(c.starters, flatnetwork, core, apps, networking, batch)
	return c
}

func (w *Context) OnLeader(f func(ctx context.Context) error) {
	w.leadership.OnLeader(f)
}

func (c *Context) WaitForCacheSyncOrDie(ctx context.Context) {
	if err := c.ControllerFactory.SharedCacheFactory().Start(ctx); err != nil {
		logrus.Fatalf("failed to start shared cache factory: %v", err)
	}
	c.ControllerFactory.SharedCacheFactory().WaitForCacheSync(ctx)
	logrus.Infof("informer cache synced")
}

// Run starts the leader-election process and block.
func (c *Context) Run(ctx context.Context) {
	c.controllerLock.Lock()
	c.leadership.Start(ctx)
	c.controllerLock.Unlock()

	logrus.Infof("waiting for pod becomes leader")
	select {}
}

func (c *Context) StartHandler(ctx context.Context, worker int) error {
	c.controllerLock.Lock()
	defer c.controllerLock.Unlock()

	return start.All(ctx, worker, c.starters...)
}
