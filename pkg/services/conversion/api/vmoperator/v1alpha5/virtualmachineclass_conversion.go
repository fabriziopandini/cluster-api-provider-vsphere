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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineClassConvertibleWrapper struct{}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineClassConvertibleWrapper{}

func (c *VirtualMachineClassConvertibleWrapper) GroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineClass")
}

func (c *VirtualMachineClassConvertibleWrapper) ConvertTo(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprv1alpha5.VirtualMachineClass)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineClass{}, srcRaw)
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineClass)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprvhub.VirtualMachineClass{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Hardware = vmoprvhub.VirtualMachineClassHardware{
		Cpus:   src.Spec.Hardware.Cpus,
		Memory: src.Spec.Hardware.Memory,
	}

	// The hub should keep track of the spoke version it was generated from.
	dst.Convertible = vmoprconversionmeta.TypeMetaConvertible{
		APIVersion: c.GroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineClassConvertibleWrapper) ConvertFrom(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineClass)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprvhub.VirtualMachineClass{}, srcRaw)
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Convertible.APIVersion != "" && src.Convertible.APIVersion != c.GroupVersionKind().GroupVersion().String() {
		errors.Errorf("src object originated from %s, it can't be converted to %s", src.Convertible.APIVersion, c.GroupVersionKind().GroupVersion().String())
	}

	dst, ok := dstRaw.(*vmoprv1alpha5.VirtualMachineClass)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineClass{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Hardware = vmoprv1alpha5.VirtualMachineClassHardware{
		Cpus:   src.Spec.Hardware.Cpus,
		Memory: src.Spec.Hardware.Memory,
	}

	return nil
}
