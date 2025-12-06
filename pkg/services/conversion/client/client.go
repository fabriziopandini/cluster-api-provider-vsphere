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
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	vmoprv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprv1alpha2conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/v1alpha2"
	vmoprv1alpha5conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/v1alpha5"
)

func NewClient(c client.Client) client.Client {
	return &conversionClient{
		internalClient: c,
	}
}

type conversionClient struct {
	internalClient client.Client

	overrideGetPreferredVersion func() string
}

// conversionClient must implement client.Client.
var _ client.Client = &conversionClient{}

func (c conversionClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Get(ctx, key, obj, opts...)
	}

	hubObj, ok := obj.(conversion.Hub)
	if !ok {
		return errors.New("obj must implement conversion.Hub")
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	vObjRaw, err := c.internalClient.Scheme().New(converter.GroupVersionKind())
	if err != nil {
		return err
	}

	vObj, ok := vObjRaw.(client.Object)
	if !ok {
		// FIXME
	}

	if err := c.internalClient.Get(ctx, key, vObj, opts...); err != nil {
		return err
	}

	converter.Set(vObj)
	return converter.ConvertTo(hubObj)
}

func (c conversionClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvkList, err := c.GroupVersionKindFor(list)
	if err != nil {
		return err
	}

	if !conversionRequired(gvkList) {
		return c.internalClient.List(ctx, list, opts...)
	}

	// FIXME: check suffix
	gvkItem := schema.GroupVersionKind{
		Group:   gvkList.Group,
		Version: gvkList.Version,
		Kind:    strings.TrimSuffix(gvkList.Kind, "List"),
	}

	// FIXME: think about how to pass explicit convertible version (field? option?)
	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvkItem, preferredVersion)
	if err != nil {
		return err
	}

	gvkVItem := converter.GroupVersionKind()
	gvkVList := schema.GroupVersionKind{
		Group:   gvkVItem.Group,
		Version: gvkVItem.Version,
		Kind:    fmt.Sprintf("%sList", gvkVItem.Kind),
	}

	vListRaw, err := c.internalClient.Scheme().New(gvkVList)
	if err != nil {
		return err
	}

	vList, ok := vListRaw.(client.ObjectList)
	if !ok {
		// FIXME
	}

	if err := c.internalClient.List(ctx, vList, opts...); err != nil {
		return err
	}

	vItems, err := meta.ExtractList(vList)
	if err != nil {
		return err
	}

	listObjs := []runtime.Object{}
	for _, vItemRaw := range vItems {
		vItem, ok := vItemRaw.(client.Object)
		if !ok {
			// FIXME
		}
		converter.Set(vItem)

		hubRaw, err := c.internalClient.Scheme().New(gvkItem)
		if err != nil {
			return err
		}

		hubObj, ok := hubRaw.(conversion.Hub)
		if !ok {
			return errors.New("list.Items must implement conversion.Hub")
		}

		converter.Set(vItem)
		if converter.ConvertTo(hubObj); err != nil {
			return err
		}
		listObjs = append(listObjs, hubObj)
	}

	if meta.SetList(list, listObjs); err != nil {
		return err
	}
	return nil
}

func (c conversionClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	// FIXME
	panic("implement me")
}

func (c conversionClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {

	// convert from hub type to vm-operator preferred version.

	// Create

	// convert from vm-operator preferred version to hub version.

	// FIXME
	panic("implement me")
}

func (c conversionClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	// FIXME
	panic("implement me")
}

func (c conversionClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// FIXME
	panic("implement me")
}

func (c conversionClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {

	// convert from hub type to vm-operator preferred version.

	// Create

	// convert from vm-operator preferred version to hub version.

	panic("implement me")
}

func (c conversionClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	// CAPV never use DeleteAllOf.
	panic("not implemented")
}

func (c conversionClient) Status() client.SubResourceWriter {
	// CAPV should not modify status of vm-operator resources.
	panic("not implemented")
}

func (c conversionClient) SubResource(_ string) client.SubResourceClient {
	// CAPV never acts on vm-operator sub-resources.
	panic("not implemented")
}

func (c conversionClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.internalClient.GroupVersionKindFor(obj)
}

func (c conversionClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.internalClient.IsObjectNamespaced(obj)
}

func (c conversionClient) Scheme() *runtime.Scheme {
	return c.internalClient.Scheme()
}

func (c conversionClient) RESTMapper() meta.RESTMapper {
	return c.internalClient.RESTMapper()
}

func (c conversionClient) preferredVersion() string {
	if c.overrideGetPreferredVersion != nil {
		return c.overrideGetPreferredVersion()
	}

	// FIXME
	panic("implement me")
}

func conversionRequired(gvk schema.GroupVersionKind) bool {
	switch gvk.GroupVersion() {
	case vmoprvhub.GroupVersion:
		return true
	}
	return false
}

func converterFor(gvk schema.GroupVersionKind, preferredVersion string) (conversion.ConvertibleWrapper, error) {
	switch preferredVersion {
	case vmoprv1alpha2.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha2conversion.VirtualMachineConvertibleWrapper{}, nil
		}
	case vmoprv1alpha5.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha5conversion.VirtualMachineConvertibleWrapper{}, nil
		}
	}
	return nil, errors.Errorf("unsupported GroupVersionKind: %s", gvk)
}
