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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
)

// MergeFrom creates a Patch that patches using the merge-patch strategy with the given object as base.
// When required, the generated patch performs conversion for both/one of the original or the target object.
func MergeFrom(c client.Client, obj client.Object) (client.Patch, error) {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return nil, err
	}

	if !conversionRequired(gvk) {
		return client.MergeFrom(obj), nil
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

	return &conversionMergePatch{
		patchType: types.MergePatchType,
		from:      obj,
		scheme:    cc.Scheme(),
		converter: converter,
	}, nil
}

type conversionMergePatch struct {
	patchType types.PatchType
	from      client.Object
	opts      []client.MergeFromOption

	scheme    *runtime.Scheme
	converter conversion.ConvertibleWrapper
}

// conversionClient must implement client.Patch.
var _ client.Patch = &conversionMergePatch{}

// Type is the PatchType of the patch.
func (p *conversionMergePatch) Type() types.PatchType {
	return p.patchType
}

// Data is the raw data representing the patch.
// Note: in case conversion are required, obj can be either the hub version or a spoke version.
func (p *conversionMergePatch) Data(obj client.Object) ([]byte, error) {
	gvkFrom, err := apiutil.GVKForObject(p.from, p.scheme)
	if err != nil {
		return nil, err
	}

	gvkTo, err := apiutil.GVKForObject(obj, p.scheme)
	if err != nil {
		return nil, err
	}

	if !conversionRequired(gvkFrom) && !conversionRequired(gvkTo) {
		if p.patchType == types.StrategicMergePatchType {
			return client.StrategicMergeFrom(p.from, p.opts...).Data(obj)
		}
		return client.MergeFromWithOptions(p.from, p.opts...).Data(obj)
	}

	fromObj := p.from
	if conversionRequired(gvkFrom) {
		spokeFromObjRaw, err := p.scheme.New(p.converter.SpokeGroupVersionKind())
		if err != nil {
			return nil, err
		}

		var ok bool
		fromObj, ok = spokeFromObjRaw.(client.Object)
		if !ok {
			return nil, errors.Errorf("%T does not implement client.Object", fromObj)
		}
		if err := p.converter.ConvertFromHub(p.from, fromObj); err != nil {
			return nil, err
		}
	}

	toObj := obj
	if conversionRequired(gvkTo) {
		spokeToObjRaw, err := p.scheme.New(p.converter.SpokeGroupVersionKind())
		if err != nil {
			return nil, err
		}

		var ok bool
		toObj, ok = spokeToObjRaw.(client.Object)
		if !ok {
			return nil, errors.Errorf("%T does not implement client.Object", toObj)
		}
		if err := p.converter.ConvertFromHub(obj, toObj); err != nil {
			return nil, err
		}
	}

	if p.patchType == types.StrategicMergePatchType {
		return client.StrategicMergeFrom(fromObj, p.opts...).Data(toObj)
	}
	return client.MergeFromWithOptions(fromObj, p.opts...).Data(toObj)
}
