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

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprv1alpha2conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/v1alpha2"
	vmoprv1alpha5conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/v1alpha5"
)

func New(c client.Client) client.Client {
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

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := c.newObj(converter.GroupVersionKind())
	if err != nil {
		return err
	}

	if err := c.internalClient.Get(ctx, key, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertTo(spokeObj, obj)
}

func (c conversionClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	hubListGVK, err := c.GroupVersionKindFor(list)
	if err != nil {
		return err
	}

	if !conversionRequired(hubListGVK) {
		return c.internalClient.List(ctx, list, opts...)
	}

	// FIXME: check suffix
	hubItemGVK := schema.GroupVersionKind{
		Group:   hubListGVK.Group,
		Version: hubListGVK.Version,
		Kind:    strings.TrimSuffix(hubListGVK.Kind, "List"),
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(hubItemGVK, preferredVersion)
	if err != nil {
		return err
	}

	spokeItemGVK := converter.GroupVersionKind()
	spokeItemList := schema.GroupVersionKind{
		Group:   spokeItemGVK.Group,
		Version: spokeItemGVK.Version,
		Kind:    fmt.Sprintf("%sList", spokeItemGVK.Kind),
	}

	spokeListRaw, err := c.internalClient.Scheme().New(spokeItemList)
	if err != nil {
		return err
	}

	spokeList, ok := spokeListRaw.(client.ObjectList)
	if !ok {
		return errors.Errorf("%T does not implement client.ObjectList", spokeList)
	}

	if err := c.internalClient.List(ctx, spokeList, opts...); err != nil {
		return err
	}

	spokeItems, err := meta.ExtractList(spokeList)
	if err != nil {
		return err
	}

	listObjs := []runtime.Object{}
	for _, spokeItemRaw := range spokeItems {
		spokeItem, ok := spokeItemRaw.(client.Object)
		if !ok {
			return errors.Errorf("%T does not implement client.Object", spokeItemRaw)
		}

		hubItem, err := c.newObj(hubItemGVK)
		if err != nil {
			return err
		}

		if converter.ConvertTo(spokeItem, hubItem); err != nil {
			return err
		}
		listObjs = append(listObjs, hubItem)
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
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Create(ctx, obj, opts...)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := c.newObj(converter.GroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFrom(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.Create(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertTo(spokeObj, obj)
}

func (c conversionClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Delete(ctx, obj, opts...)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := c.newObj(converter.GroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFrom(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.Delete(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertTo(spokeObj, obj)
}

func (c conversionClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Update(ctx, obj, opts...)
	}

	panic("implement me")
}

func (c conversionClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Patch(ctx, obj, patch, opts...)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := c.newObj(converter.GroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFrom(obj, spokeObj); err != nil {
		return err
	}

	hubPatch, ok := patch.(Patch)
	if !ok {
		return errors.Errorf("%T does not implement conversion.client.Patch", patch)
	}

	spokePatch, err := hubPatch.ConversionPatch(c.internalClient.Scheme(), converter)
	if err != nil {
		return err
	}

	if err := c.internalClient.Patch(ctx, spokeObj, spokePatch, opts...); err != nil {
		return err
	}
	return converter.ConvertTo(spokeObj, obj)
}

func (c conversionClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.DeleteAllOf(ctx, obj, opts...)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := c.newObj(converter.GroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFrom(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.DeleteAllOf(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertTo(spokeObj, obj)
}

func (c conversionClient) Status() client.SubResourceWriter {
	// FIXME: looks like there is no way to prevent this for the hub version (not sure we have / want to block)
	return c.internalClient.Status()
}

func (c conversionClient) SubResource(subResource string) client.SubResourceClient {
	return c.internalClient.SubResource(subResource)
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

	// FIXME: Add the logic to detect the resource version in a live system.
	return vmoprv1alpha2.GroupVersion.Version
}

func conversionRequired(gvk schema.GroupVersionKind) bool {
	switch gvk.GroupVersion() {
	case vmoprvhub.GroupVersion:
		return true
	}
	return false
}

// FIXME: implement test to check all the GVK/preferred versions have a converter
func converterFor(gvk schema.GroupVersionKind, preferredVersion string) (vmoprconversion.ConvertibleWrapper, error) {
	switch preferredVersion {
	case vmoprv1alpha2.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha2conversion.VirtualMachineConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineClass"):
			return &vmoprv1alpha2conversion.VirtualMachineClassConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineImage"):
			return &vmoprv1alpha2conversion.VirtualMachineImageConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineService"):
			return &vmoprv1alpha2conversion.VirtualMachineServiceConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineSetResourcePolicy"):
			return &vmoprv1alpha2conversion.VirtualMachineSetResourcePolicyConvertibleWrapper{}, nil
		}
	case vmoprv1alpha5.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha5conversion.VirtualMachineConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineClass"):
			return &vmoprv1alpha5conversion.VirtualMachineClassConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineImage"):
			return &vmoprv1alpha5conversion.VirtualMachineImageConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineService"):
			return &vmoprv1alpha5conversion.VirtualMachineServiceConvertibleWrapper{}, nil
		case vmoprvhub.GroupVersion.WithKind("VirtualMachineSetResourcePolicy"):
			return &vmoprv1alpha5conversion.VirtualMachineSetResourcePolicyConvertibleWrapper{}, nil
		}
	}
	return nil, errors.Errorf("can't find a converter for %s", gvk)
}

func (c conversionClient) newObj(gvk schema.GroupVersionKind) (client.Object, error) {
	vObjRaw, err := c.internalClient.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}

	vObj, ok := vObjRaw.(client.Object)
	if !ok {
		return nil, errors.Errorf("%T does not implement client.Object", vObjRaw)
	}
	return vObj, nil
}
