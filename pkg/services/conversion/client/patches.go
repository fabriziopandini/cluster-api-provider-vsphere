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

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
)

// Patch is a patch that can be applied to a Kubernetes object.
type Patch interface {
	client.Patch

	ConversionPatch(scheme *runtime.Scheme, converter vmoprconversion.ConvertibleWrapper) (client.Patch, error)
}

type mergeFromPatch struct {
	client.Patch
	patchType types.PatchType
	from      client.Object
	opts      []client.MergeFromOption
}

func MergeFrom(obj client.Object) client.Patch {
	return &mergeFromPatch{Patch: client.MergeFrom(obj), patchType: types.MergePatchType, from: obj}
}

func MergeFromWithOptions(obj client.Object, opts ...client.MergeFromOption) client.Patch {
	return &mergeFromPatch{Patch: client.MergeFromWithOptions(obj, opts...), patchType: types.MergePatchType, from: obj, opts: opts}
}

func StrategicMergeFrom(obj client.Object, opts ...client.MergeFromOption) client.Patch {
	return &mergeFromPatch{Patch: client.MergeFromWithOptions(obj, opts...), patchType: types.StrategicMergePatchType, from: obj, opts: opts}
}

func (p *mergeFromPatch) ConversionPatch(scheme *runtime.Scheme, converter vmoprconversion.ConvertibleWrapper) (client.Patch, error) {
	hubFromObj, ok := p.from.(vmoprconversion.Hub)
	if !ok {
		return nil, errors.New("obj must implement conversion.Hub")
	}

	spokeFromObjRaw, err := scheme.New(converter.GroupVersionKind())
	if err != nil {
		return nil, err
	}

	spokeFromObj, ok := spokeFromObjRaw.(client.Object)
	if !ok {
		// FIXME
	}
	if err := converter.ConvertFrom(hubFromObj, spokeFromObj); err != nil {
		// FIXME:
	}

	if p.patchType == types.StrategicMergePatchType {
		return client.StrategicMergeFrom(spokeFromObj, p.opts...), nil
	}
	if len(p.opts) > 0 {
		return client.MergeFromWithOptions(spokeFromObj, p.opts...), nil
	}
	return client.MergeFrom(spokeFromObj), nil
}
