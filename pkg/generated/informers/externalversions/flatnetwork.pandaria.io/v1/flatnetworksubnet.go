/*
Copyright 2025 SUSE Rancher

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
	context "context"
	time "time"

	apisflatnetworkpandariaiov1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	versioned "github.com/cnrancher/rancher-flat-network/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/cnrancher/rancher-flat-network/pkg/generated/informers/externalversions/internalinterfaces"
	flatnetworkpandariaiov1 "github.com/cnrancher/rancher-flat-network/pkg/generated/listers/flatnetwork.pandaria.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// FlatNetworkSubnetInformer provides access to a shared informer and lister for
// FlatNetworkSubnets.
type FlatNetworkSubnetInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() flatnetworkpandariaiov1.FlatNetworkSubnetLister
}

type flatNetworkSubnetInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewFlatNetworkSubnetInformer constructs a new informer for FlatNetworkSubnet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFlatNetworkSubnetInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredFlatNetworkSubnetInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredFlatNetworkSubnetInformer constructs a new informer for FlatNetworkSubnet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredFlatNetworkSubnetInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.FlatnetworkV1().FlatNetworkSubnets(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.FlatnetworkV1().FlatNetworkSubnets(namespace).Watch(context.TODO(), options)
			},
		},
		&apisflatnetworkpandariaiov1.FlatNetworkSubnet{},
		resyncPeriod,
		indexers,
	)
}

func (f *flatNetworkSubnetInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredFlatNetworkSubnetInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *flatNetworkSubnetInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&apisflatnetworkpandariaiov1.FlatNetworkSubnet{}, f.defaultInformer)
}

func (f *flatNetworkSubnetInformer) Lister() flatnetworkpandariaiov1.FlatNetworkSubnetLister {
	return flatnetworkpandariaiov1.NewFlatNetworkSubnetLister(f.Informer().GetIndexer())
}
