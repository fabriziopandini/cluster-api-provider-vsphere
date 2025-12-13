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

// MergePatchCreator is a client that can create merge or strategic merge patch.
// FIXME: Pluggable in CABPK/KCP & Patch helper.
type MergePatchCreator interface {
	// MergeFrom creates a Patch that patches using the merge-patch strategy with the given object as base.
	MergeFrom(obj client.Object) (client.Patch, error)

	// MergeFromWithOptions creates a Patch that patches using the merge-patch strategy with the given object as base.
	MergeFromWithOptions(obj client.Object, opts ...client.MergeFromOption) (client.Patch, error)

	// StrategicMergeFrom creates a Patch that patches using the strategic-merge-patch strategy with the given object as base.
	StrategicMergeFrom(obj client.Object, opts ...client.MergeFromOption) (client.Patch, error)
}

// conversionClient must implement WatchObjectCreator.
var _ MergePatchCreator = &conversionClient{}

// MergeFrom creates a Patch that patches using the merge-patch strategy with the given object as base.
func (c conversionClient) MergeFrom(obj client.Object) (client.Patch, error) {
	return c.newMergePatch(types.MergePatchType, obj)
}

// MergeFromWithOptions creates a Patch that patches using the merge-patch strategy with the given object as base.
func (c conversionClient) MergeFromWithOptions(obj client.Object, opts ...client.MergeFromOption) (client.Patch, error) {
	return c.newMergePatch(types.MergePatchType, obj, opts...)
}

// StrategicMergeFrom creates a Patch that patches using the strategic-merge-patch strategy with the given object as base.
func (c conversionClient) StrategicMergeFrom(obj client.Object, opts ...client.MergeFromOption) (client.Patch, error) {
	return c.newMergePatch(types.StrategicMergePatchType, obj, opts...)
}

func (c conversionClient) newMergePatch(patchType types.PatchType, obj client.Object, opts ...client.MergeFromOption) (client.Patch, error) {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return nil, err
	}

	if !conversionRequired(gvk) {
		if patchType == types.StrategicMergePatchType {
			return client.StrategicMergeFrom(obj, opts...), nil
		}
		return client.MergeFromWithOptions(obj, opts...), nil
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return nil, err
	}

	return &conversionMergePatch{
		patchType: patchType,
		from:      obj,
		opts:      opts,
		scheme:    c.Scheme(),
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
func (p *conversionMergePatch) Data(obj client.Object) ([]byte, error) {
	gvk, err := apiutil.GVKForObject(obj, p.scheme)
	if err != nil {
		return nil, err
	}

	if !conversionRequired(gvk) {
		if p.patchType == types.StrategicMergePatchType {
			return client.StrategicMergeFrom(p.from, p.opts...).Data(obj)
		}
		return client.MergeFromWithOptions(p.from, p.opts...).Data(obj)
	}

	spokeFromObjRaw, err := p.scheme.New(p.converter.SpokeGroupVersionKind())
	if err != nil {
		return nil, err
	}

	spokeFromObj, ok := spokeFromObjRaw.(client.Object)
	if !ok {
		return nil, errors.Errorf("%T does not implement client.Object", spokeFromObjRaw)
	}
	if err := p.converter.ConvertFromHub(p.from, spokeFromObj); err != nil {
		return nil, err
	}

	if p.patchType == types.StrategicMergePatchType {
		return client.StrategicMergeFrom(spokeFromObj, p.opts...).Data(obj)
	}
	return client.MergeFromWithOptions(spokeFromObj, p.opts...).Data(obj)
}
