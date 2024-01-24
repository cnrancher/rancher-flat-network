package main

import (
	"fmt"
	"os"
	"path"

	v12 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	os.Unsetenv("GOPATH")

	controllergen.Run(args.Options{
		OutputPackage: "github.com/cnrancher/macvlan-operator/pkg/generated",
		Boilerplate:   "pkg/codegen/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"macvlan.cluster.cattle.io": {
				Types: []any{
					macvlanv1.MacvlanIP{},
					macvlanv1.MacvlanSubnet{},
				},
				GenerateTypes:     true,
				GenerateClients:   true,
				GenerateListers:   true,
				GenerateInformers: true,
			},
			corev1.GroupName: {
				Types: []any{
					corev1.Namespace{},
					corev1.Service{},
					corev1.Pod{},
					corev1.Event{},
					// corev1.Node{},
					// corev1.Secret{},
					// corev1.ServiceAccount{},
					// corev1.Endpoints{},
					// corev1.ConfigMap{},
					// corev1.PersistentVolumeClaim{},
				},
				// InformersPackage: "k8s.io/client-go/informers",
				// ClientSetPackage: "k8s.io/client-go/kubernetes",
				// ListersPackage:   "k8s.io/client-go/listers",
			},
			appsv1.GroupName: {
				Types: []any{
					appsv1.Deployment{},
					appsv1.DaemonSet{},
					appsv1.StatefulSet{},
					appsv1.ReplicaSet{},
				},
				// InformersPackage: "k8s.io/client-go/informers",
				// ClientSetPackage: "k8s.io/client-go/kubernetes",
				// ListersPackage:   "k8s.io/client-go/listers",
			},
			batchv1.GroupName: {
				Types: []any{
					batchv1.CronJob{},
					batchv1.Job{},
				},
				// InformersPackage: "k8s.io/client-go/informers",
				// ClientSetPackage: "k8s.io/client-go/kubernetes",
				// ListersPackage:   "k8s.io/client-go/listers",
			},
		},
	})

	var crds []crd.CRD

	ipConfig := newCRD(&v12.MacvlanIP{}, func(c crd.CRD) crd.CRD {
		c.ShortNames = []string{
			"mip",
			"mips",
		}
		return c
	})
	subnetConfig := newCRD(&v12.MacvlanSubnet{}, func(c crd.CRD) crd.CRD {
		c.ShortNames = []string{
			"msubnet",
			"msubnets",
		}
		return c
	})
	crds = append(crds, ipConfig, subnetConfig)

	var data []byte
	for _, crd := range crds {
		obj, err := crd.ToCustomResourceDefinition()
		if err != nil {
			panic(err)
		}
		obj.(*unstructured.Unstructured).SetAnnotations(map[string]string{
			"helm.sh/resource-policy": "keep",
		})
		b, err := yaml.Export(obj)
		if err != nil {
			panic(err)
		}
		data = append(data, []byte("---\n")...)
		data = append(data, b...)
	}
	if err := saveCRDYaml("macvlan-operator-crd", string(data)); err != nil {
		panic(err)
	}
}

func newCRD(obj any, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "macvlan.cluster.cattle.io",
			Version: "v1",
		},
		Status:       true,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}

func saveCRDYaml(name, data string) error {
	filepath := fmt.Sprintf("./charts/%s/templates/", name)
	if err := os.MkdirAll(filepath, 0755); err != nil {
		return fmt.Errorf("failed to mkdir %q: %w", filepath, err)
	}

	filename := path.Join(filepath, "crds.yaml")
	if err := os.WriteFile(filename, []byte(data), 0644); err != nil {
		return err
	}

	return nil
}
