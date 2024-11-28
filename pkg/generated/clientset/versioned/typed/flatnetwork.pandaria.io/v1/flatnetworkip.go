/*
Copyright 2024 SUSE Rancher

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package v1

import (
	"context"

	v1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	scheme "github.com/cnrancher/rancher-flat-network/pkg/generated/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// FlatNetworkIPsGetter has a method to return a FlatNetworkIPInterface.
// A group's client should implement this interface.
type FlatNetworkIPsGetter interface {
	FlatNetworkIPs(namespace string) FlatNetworkIPInterface
}

// FlatNetworkIPInterface has methods to work with FlatNetworkIP resources.
type FlatNetworkIPInterface interface {
	Create(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.CreateOptions) (*v1.FlatNetworkIP, error)
	Update(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.UpdateOptions) (*v1.FlatNetworkIP, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.UpdateOptions) (*v1.FlatNetworkIP, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.FlatNetworkIP, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.FlatNetworkIPList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.FlatNetworkIP, err error)
	FlatNetworkIPExpansion
}

// flatNetworkIPs implements FlatNetworkIPInterface
type flatNetworkIPs struct {
	*gentype.ClientWithList[*v1.FlatNetworkIP, *v1.FlatNetworkIPList]
}

// newFlatNetworkIPs returns a FlatNetworkIPs
func newFlatNetworkIPs(c *FlatnetworkV1Client, namespace string) *flatNetworkIPs {
	return &flatNetworkIPs{
		gentype.NewClientWithList[*v1.FlatNetworkIP, *v1.FlatNetworkIPList](
			"flatnetworkips",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1.FlatNetworkIP { return &v1.FlatNetworkIP{} },
			func() *v1.FlatNetworkIPList { return &v1.FlatNetworkIPList{} }),
	}
}
