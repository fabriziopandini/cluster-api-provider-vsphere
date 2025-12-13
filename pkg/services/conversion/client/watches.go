/*
Copyright 2025 The Kubernetes Authors.

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

package client

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WatchObjectCreator is a client that can create objects to be used for building controller watches.
// FIXME: Pluggable in CABPK/KCP.
type WatchObjectCreator interface {
	// NewWatchObject creates an object to be used for  building watches.
	NewWatchObject(gvk schema.GroupVersionKind) (client.Object, error)
}

// NewWatchObject creates an object to be used for  building controller watches.
// FIXME: Move to conversion? (may be also create alias for MergeFrom etc).
func NewWatchObject(c client.Client, gvk schema.GroupVersionKind) (client.Object, error) {
	if watchObjectCreator, ok := c.(WatchObjectCreator); ok {
		return watchObjectCreator.NewWatchObject(gvk)
	}
	return newClientObject(c.Scheme(), gvk)
}

// conversionClient must implement WatchObjectCreator.
var _ WatchObjectCreator = &conversionClient{}

// Returns an object to be used for  building controller watches.
func (c conversionClient) NewWatchObject(gvk schema.GroupVersionKind) (client.Object, error) {
	if !conversionRequired(gvk) {
		return newClientObject(c.internalClient.Scheme(), gvk)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return nil, err
	}

	return newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
}
