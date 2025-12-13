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

// New return a new conversion aware client.
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

// Get retrieves an obj for the given object key from the Kubernetes Cluster.
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

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}

	if err := c.internalClient.Get(ctx, key, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, obj)
}

// List retrieves list of objects for a given namespace and list options.
func (c conversionClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	hubListGVK, err := c.GroupVersionKindFor(list)
	if err != nil {
		return err
	}

	if !conversionRequired(hubListGVK) {
		return c.internalClient.List(ctx, list, opts...)
	}

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

	spokeItemGVK := converter.SpokeGroupVersionKind()
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

		hubItem, err := newClientObject(c.internalClient.Scheme(), hubItemGVK)
		if err != nil {
			return err
		}

		if err := converter.ConvertToHub(spokeItem, hubItem); err != nil {
			return err
		}
		listObjs = append(listObjs, hubItem)
	}

	return meta.SetList(list, listObjs)
}

// Apply applies the given apply configuration to the Kubernetes cluster.
func (c conversionClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	cObj, ok := obj.(client.Object)
	if !ok {
		return errors.Errorf("%T does not implement client.Object", obj)
	}

	gvk, err := c.GroupVersionKindFor(cObj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Apply(ctx, obj, opts...)
	}

	preferredVersion := c.preferredVersion()
	converter, err := converterFor(gvk, preferredVersion)
	if err != nil {
		return err
	}

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFromHub(cObj, spokeObj); err != nil {
		return err
	}

	spokeApplyConfiguration, ok := spokeObj.(runtime.ApplyConfiguration)
	if !ok {
		return errors.Errorf("%T does not implement runtime.ApplyConfiguration", spokeObj)
	}

	if err := c.internalClient.Apply(ctx, spokeApplyConfiguration, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, cObj)
}

// Create saves the object obj in the Kubernetes cluster.
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

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFromHub(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.Create(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, obj)
}

// Delete deletes the given obj from Kubernetes cluster.
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

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFromHub(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.Delete(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, obj)
}

// Update updates the given obj in the Kubernetes cluster.
func (c conversionClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	if !conversionRequired(gvk) {
		return c.internalClient.Update(ctx, obj, opts...)
	}

	return errors.New("Update must not be used when conversion is required. Use patch instead")
}

// Patch patches the given obj in the Kubernetes cluster.
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

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFromHub(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.Patch(ctx, spokeObj, patch, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, obj)
}

// DeleteAllOf deletes all objects of the given type matching the given options.
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

	spokeObj, err := newClientObject(c.internalClient.Scheme(), converter.SpokeGroupVersionKind())
	if err != nil {
		return err
	}
	if err := converter.ConvertFromHub(obj, spokeObj); err != nil {
		return err
	}

	if err := c.internalClient.DeleteAllOf(ctx, spokeObj, opts...); err != nil {
		return err
	}
	return converter.ConvertToHub(spokeObj, obj)
}

func (c conversionClient) Status() client.SubResourceWriter {
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
	return gvk.GroupVersion() == vmoprvhub.GroupVersion
}

func converterFor(gvk schema.GroupVersionKind, preferredVersion string) (conversion.ConvertibleWrapper, error) {
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
	return nil, errors.Errorf("can't find a converter from %s to %s", gvk, preferredVersion)
}

func newClientObject(s *runtime.Scheme, gvk schema.GroupVersionKind) (client.Object, error) {
	vObjRaw, err := s.New(gvk)
	if err != nil {
		return nil, err
	}

	vObj, ok := vObjRaw.(client.Object)
	if !ok {
		return nil, errors.Errorf("%T does not implement client.Object", vObjRaw)
	}
	return vObj, nil
}
