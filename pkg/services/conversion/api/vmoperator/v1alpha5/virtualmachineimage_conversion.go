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

package v1alpha5

import "C"
import (
	"github.com/pkg/errors"
	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmoprv1alpha5common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineImageConvertibleWrapper struct{}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineImageConvertibleWrapper{}

func (c *VirtualMachineImageConvertibleWrapper) SpokeGroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineImage")
}

func (c *VirtualMachineImageConvertibleWrapper) ConvertToHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprv1alpha5.VirtualMachineImage)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineImage{}, srcRaw)
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineImage)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprvhub.VirtualMachineImage{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.ProviderRef != nil {
		dst.Spec.ProviderRef = &vmoprvhub.LocalObjectRef{
			APIVersion: src.Spec.ProviderRef.APIVersion,
			Kind:       src.Spec.ProviderRef.Kind,
			Name:       src.Spec.ProviderRef.Name,
		}
	}

	if src.Status.Conditions != nil {
		dst.Status.Conditions = []metav1.Condition{}
		for _, condition := range src.Status.Conditions {
			dst.Status.Conditions = append(dst.Status.Conditions, condition)
		}
	}
	dst.Status.Name = src.Status.Name
	dst.Status.OSInfo = vmoprvhub.VirtualMachineImageOSInfo{
		Type: src.Status.OSInfo.Type,
	}
	dst.Status.ProductInfo = vmoprvhub.VirtualMachineImageProductInfo{
		FullVersion: src.Status.ProductInfo.FullVersion,
	}
	dst.Status.ProviderItemID = src.Status.ProviderItemID

	// The hub should keep track of the spoke version it was generated from.
	dst.Source = vmoprconversionmeta.SourceTypeMeta{
		APIVersion: c.SpokeGroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineImageConvertibleWrapper) ConvertFromHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineImage)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprvhub.VirtualMachineImage{}, srcRaw)
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Source.APIVersion != "" && src.Source.APIVersion != c.SpokeGroupVersionKind().GroupVersion().String() {
		errors.Errorf("src object originated from %s, it can't be converted to %s", src.Source.APIVersion, c.SpokeGroupVersionKind().GroupVersion().String())
	}

	dst, ok := dstRaw.(*vmoprv1alpha5.VirtualMachineImage)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineImage{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.ProviderRef != nil {
		dst.Spec.ProviderRef = &vmoprv1alpha5common.LocalObjectRef{
			APIVersion: src.Spec.ProviderRef.APIVersion,
			Kind:       src.Spec.ProviderRef.Kind,
			Name:       src.Spec.ProviderRef.Name,
		}
	}

	if src.Status.Conditions != nil {
		dst.Status.Conditions = []metav1.Condition{}
		for _, condition := range src.Status.Conditions {
			dst.Status.Conditions = append(dst.Status.Conditions, condition)
		}
	}
	dst.Status.Name = src.Status.Name
	dst.Status.OSInfo = vmoprv1alpha5.VirtualMachineImageOSInfo{
		Type: src.Status.OSInfo.Type,
	}
	dst.Status.ProductInfo = vmoprv1alpha5.VirtualMachineImageProductInfo{
		FullVersion: src.Status.ProductInfo.FullVersion,
	}
	dst.Status.ProviderItemID = src.Status.ProviderItemID

	return nil
}
