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
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WatchObject creates an object to be used for building controller watches.
// When required, the generated object is the converted version of the input object.
func WatchObject(c client.Client, obj client.Object) (client.Object, error) {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return nil, err
	}

	if !conversionRequired(gvk) {
		return newClientObject(c.Scheme(), gvk)
	}

	cc, ok := c.(*conversionClient)
	if !ok {
		return nil, errors.Errorf("%T does not implement client.Object", obj)
	}

	preferredVersion := cc.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return nil, err
	}

	return newClientObject(cc.Scheme(), converter.SpokeGroupVersionKind())
}
