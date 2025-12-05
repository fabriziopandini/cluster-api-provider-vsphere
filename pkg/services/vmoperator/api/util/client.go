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

package util

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

	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/core/hub"
	vmoprv1alpha2conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/core/v1alpha2"
	vmoprv1alpha5conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/core/v1alpha5"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/util/conversion"
)

func NewVersionAwareClient(c client.Client) client.Client {
	return &versionAwareClient{
		internalClient: c,
	}
}

type versionAwareClient struct {
	internalClient client.Client

	overrideGetPreferredVersion func() string
}

// versionAwareClient must implement client.Client.
var _ client.Client = &versionAwareClient{}

func (c versionAwareClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	hubObj, ok := obj.(conversion.Hub)
	if !ok {
		return errors.New("obj must implement conversion.Hub")
	}

	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	preferredVersion := hubObj.APIVersion()
	if preferredVersion == "" {
		preferredVersion = c.preferredVersion()
	}
	converter, err := c.converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	vObjRaw, err := c.internalClient.Scheme().New(converter.GVK())
	if err != nil {
		return err
	}

	vObj, ok := vObjRaw.(client.Object)
	if !ok {
		// TODO
	}

	if err := c.internalClient.Get(ctx, key, vObj, opts...); err != nil {
		return err
	}

	converter.Set(vObj)
	return converter.ConvertTo(hubObj)
}

func (c versionAwareClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvkList, err := c.GroupVersionKindFor(list)
	if err != nil {
		return err
	}

	// TODO: check suffix
	gvkItem := schema.GroupVersionKind{
		Group:   gvkList.Group,
		Version: gvkList.Version,
		Kind:    strings.TrimSuffix(gvkList.Kind, "List"),
	}

	// FIXME: think about how to pass explicit convertible version (field? option?)
	preferredVersion := c.preferredVersion()
	converter, err := c.converterFor(gvkItem, preferredVersion)
	if err != nil {
		return err
	}

	gvkVItem := converter.GVK()
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
		// TODO
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
			// TODO
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

func (c versionAwareClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	// TODO implement me
	panic("implement me")
}

func (c versionAwareClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {

	// convert from hub type to vm-operator preferred version.

	// Create

	// convert from vm-operator preferred version to hub version.

	// TODO implement me
	panic("implement me")
}

func (c versionAwareClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	// TODO implement me
	panic("implement me")
}

func (c versionAwareClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// TODO implement me
	panic("implement me")
}

func (c versionAwareClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {

	// convert from hub type to vm-operator preferred version.

	// Create

	// convert from vm-operator preferred version to hub version.

	panic("implement me")
}

func (c versionAwareClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	// CAPV never use DeleteAllOf.
	panic("not implemented")
}

func (c versionAwareClient) Status() client.SubResourceWriter {
	// CAPV should not modify status of vm-operator resources.
	panic("not implemented")
}

func (c versionAwareClient) SubResource(_ string) client.SubResourceClient {
	// CAPV never acts on vm-operator sub-resources.
	panic("not implemented")
}

func (c versionAwareClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.internalClient.GroupVersionKindFor(obj)
}

func (c versionAwareClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.internalClient.IsObjectNamespaced(obj)
}

func (c versionAwareClient) Scheme() *runtime.Scheme {
	return c.internalClient.Scheme()
}

func (c versionAwareClient) RESTMapper() meta.RESTMapper {
	return c.internalClient.RESTMapper()
}

func (c versionAwareClient) preferredVersion() string {
	if c.overrideGetPreferredVersion != nil {
		return c.overrideGetPreferredVersion()
	}

	// TODO implement me
	panic("implement me")
}

func (c versionAwareClient) converterFor(gvk schema.GroupVersionKind, preferredVersion string) (conversion.Convertible, error) {
	switch preferredVersion {
	case vmoprv1alpha2.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha2conversion.VirtualMachineConverter{}, nil
		}
	case vmoprv1alpha5.GroupVersion.Version:
		switch gvk {
		case vmoprvhub.GroupVersion.WithKind("VirtualMachine"):
			return &vmoprv1alpha5conversion.VirtualMachineConverter{}, nil
		}
	}

	return nil, errors.Errorf("unsupported GroupVersionKind: %s", gvk)
}
