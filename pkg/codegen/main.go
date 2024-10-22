package main

import (
	"fmt"
	"os"
	"path"

	controllergen "github.com/rancher/wrangler/v3/pkg/controller-gen"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"github.com/rancher/wrangler/v3/pkg/crd"
	"github.com/rancher/wrangler/v3/pkg/yaml"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	flatnetworkv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
)

func main() {
	os.Unsetenv("GOPATH")

	controllergen.Run(args.Options{
		OutputPackage: "github.com/cnrancher/rancher-flat-network/pkg/generated",
		Boilerplate:   "pkg/codegen/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"flatnetwork.pandaria.io": {
				Types: []any{
					flatnetworkv1.FlatNetworkIP{},
					flatnetworkv1.FlatNetworkSubnet{},
				},
				GenerateTypes:     true,
				GenerateClients:   true,
				GenerateListers:   true,
				GenerateInformers: true,
			},
			corev1.GroupName: {
				Types: []any{
					corev1.Pod{},
					corev1.Service{},
					corev1.Namespace{},
					// corev1.Secret{},
					corev1.Endpoints{},
					// corev1.ConfigMap{},
				},
			},
			appsv1.GroupName: {
				Types: []any{
					appsv1.Deployment{},
					appsv1.DaemonSet{},
					appsv1.StatefulSet{},
					appsv1.ReplicaSet{},
				},
			},
			batchv1.GroupName: {
				Types: []any{
					batchv1.CronJob{},
					batchv1.Job{},
				},
			},
			discoveryv1.GroupName: {
				Types: []any{
					discoveryv1.EndpointSlice{},
				},
			},
			networkingv1.GroupName: {
				Types: []any{
					networkingv1.Ingress{},
				},
			},
		},
	})

	var crds []crd.CRD

	ipConfig := newCRD(&flatnetworkv1.FlatNetworkIP{}, func(c crd.CRD) crd.CRD {
		if c.Schema == nil {
			c.Schema = &apiextensionsv1.JSONSchemaProps{}
		}
		c.ShortNames = []string{
			"flatnetworkip",
			"flip",
			"flips",
		}
		return c
	})
	subnetConfig := newCRD(&flatnetworkv1.FlatNetworkSubnet{}, func(c crd.CRD) crd.CRD {
		if c.Schema == nil {
			c.Schema = &apiextensionsv1.JSONSchemaProps{}
		}
		c.ShortNames = []string{
			"flatnetworksubnet",
			"flsubnet",
			"flsubnets",
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
	if err := saveCRDYaml("rancher-flat-network-crd", string(data)); err != nil {
		panic(err)
	}
}

func newCRD(obj any, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "flatnetwork.pandaria.io",
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
