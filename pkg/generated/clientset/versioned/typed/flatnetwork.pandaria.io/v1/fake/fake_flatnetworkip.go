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

// FakeFlatNetworkIPs implements FlatNetworkIPInterface
type FakeFlatNetworkIPs struct {
	Fake *FakeFlatnetworkV1
	ns   string
}

var flatnetworkipsResource = v1.SchemeGroupVersion.WithResource("flatnetworkips")

var flatnetworkipsKind = v1.SchemeGroupVersion.WithKind("FlatNetworkIP")

// Get takes name of the flatNetworkIP, and returns the corresponding flatNetworkIP object, and an error if there is any.
func (c *FakeFlatNetworkIPs) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.FlatNetworkIP, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(flatnetworkipsResource, c.ns, name), &v1.FlatNetworkIP{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkIP), err
}

// List takes label and field selectors, and returns the list of FlatNetworkIPs that match those selectors.
func (c *FakeFlatNetworkIPs) List(ctx context.Context, opts metav1.ListOptions) (result *v1.FlatNetworkIPList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(flatnetworkipsResource, flatnetworkipsKind, c.ns, opts), &v1.FlatNetworkIPList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.FlatNetworkIPList{ListMeta: obj.(*v1.FlatNetworkIPList).ListMeta}
	for _, item := range obj.(*v1.FlatNetworkIPList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested flatNetworkIPs.
func (c *FakeFlatNetworkIPs) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(flatnetworkipsResource, c.ns, opts))

}

// Create takes the representation of a flatNetworkIP and creates it.  Returns the server's representation of the flatNetworkIP, and an error, if there is any.
func (c *FakeFlatNetworkIPs) Create(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.CreateOptions) (result *v1.FlatNetworkIP, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(flatnetworkipsResource, c.ns, flatNetworkIP), &v1.FlatNetworkIP{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkIP), err
}

// Update takes the representation of a flatNetworkIP and updates it. Returns the server's representation of the flatNetworkIP, and an error, if there is any.
func (c *FakeFlatNetworkIPs) Update(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.UpdateOptions) (result *v1.FlatNetworkIP, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(flatnetworkipsResource, c.ns, flatNetworkIP), &v1.FlatNetworkIP{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkIP), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeFlatNetworkIPs) UpdateStatus(ctx context.Context, flatNetworkIP *v1.FlatNetworkIP, opts metav1.UpdateOptions) (*v1.FlatNetworkIP, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(flatnetworkipsResource, "status", c.ns, flatNetworkIP), &v1.FlatNetworkIP{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkIP), err
}

// Delete takes name of the flatNetworkIP and deletes it. Returns an error if one occurs.
func (c *FakeFlatNetworkIPs) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(flatnetworkipsResource, c.ns, name, opts), &v1.FlatNetworkIP{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeFlatNetworkIPs) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(flatnetworkipsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1.FlatNetworkIPList{})
	return err
}

// Patch applies the patch and returns the patched flatNetworkIP.
func (c *FakeFlatNetworkIPs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.FlatNetworkIP, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(flatnetworkipsResource, c.ns, name, pt, data, subresources...), &v1.FlatNetworkIP{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.FlatNetworkIP), err
}