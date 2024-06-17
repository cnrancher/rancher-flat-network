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

package fake

import (
	"context"

	v1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeFlatNetworkSubnets implements FlatNetworkSubnetInterface
type FakeFlatNetworkSubnets struct {
	Fake *FakeFlatnetworkV1
	ns   string
}

var flatnetworksubnetsResource = v1.SchemeGroupVersion.WithResource("flatnetworksubnets")

var flatnetworksubnetsKind = v1.SchemeGroupVersion.WithKind("FlatNetworkSubnet")

// Get takes name of the flatNetworkSubnet, and returns the corresponding flatNetworkSubnet object, and an error if there is any.
func (c *FakeFlatNetworkSubnets) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.FlatNetworkSubnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(flatnetworksubnetsResource, c.ns, name), &v1.FlatNetworkSubnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkSubnet), err
}

// List takes label and field selectors, and returns the list of FlatNetworkSubnets that match those selectors.
func (c *FakeFlatNetworkSubnets) List(ctx context.Context, opts metav1.ListOptions) (result *v1.FlatNetworkSubnetList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(flatnetworksubnetsResource, flatnetworksubnetsKind, c.ns, opts), &v1.FlatNetworkSubnetList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.FlatNetworkSubnetList{ListMeta: obj.(*v1.FlatNetworkSubnetList).ListMeta}
	for _, item := range obj.(*v1.FlatNetworkSubnetList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested flatNetworkSubnets.
func (c *FakeFlatNetworkSubnets) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(flatnetworksubnetsResource, c.ns, opts))

}

// Create takes the representation of a flatNetworkSubnet and creates it.  Returns the server's representation of the flatNetworkSubnet, and an error, if there is any.
func (c *FakeFlatNetworkSubnets) Create(ctx context.Context, flatNetworkSubnet *v1.FlatNetworkSubnet, opts metav1.CreateOptions) (result *v1.FlatNetworkSubnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(flatnetworksubnetsResource, c.ns, flatNetworkSubnet), &v1.FlatNetworkSubnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkSubnet), err
}

// Update takes the representation of a flatNetworkSubnet and updates it. Returns the server's representation of the flatNetworkSubnet, and an error, if there is any.
func (c *FakeFlatNetworkSubnets) Update(ctx context.Context, flatNetworkSubnet *v1.FlatNetworkSubnet, opts metav1.UpdateOptions) (result *v1.FlatNetworkSubnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(flatnetworksubnetsResource, c.ns, flatNetworkSubnet), &v1.FlatNetworkSubnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkSubnet), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeFlatNetworkSubnets) UpdateStatus(ctx context.Context, flatNetworkSubnet *v1.FlatNetworkSubnet, opts metav1.UpdateOptions) (*v1.FlatNetworkSubnet, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(flatnetworksubnetsResource, "status", c.ns, flatNetworkSubnet), &v1.FlatNetworkSubnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkSubnet), err
}

// Delete takes name of the flatNetworkSubnet and deletes it. Returns an error if one occurs.
func (c *FakeFlatNetworkSubnets) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(flatnetworksubnetsResource, c.ns, name, opts), &v1.FlatNetworkSubnet{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFlatNetworkSubnets) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(flatnetworksubnetsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1.FlatNetworkSubnetList{})
	return err
}

// Patch applies the patch and returns the patched flatNetworkSubnet.
func (c *FakeFlatNetworkSubnets) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.FlatNetworkSubnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(flatnetworksubnetsResource, c.ns, name, pt, data, subresources...), &v1.FlatNetworkSubnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkSubnet), err
}