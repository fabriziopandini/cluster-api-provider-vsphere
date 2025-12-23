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

package v1alpha2

import (
	"github.com/pkg/errors"
	vmoprv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineClassConvertibleWrapper struct{}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineClassConvertibleWrapper{}

func (c *VirtualMachineClassConvertibleWrapper) SpokeGroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha2.GroupVersion.WithKind("VirtualMachineClass")
}

func (c *VirtualMachineClassConvertibleWrapper) ConvertToHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprv1alpha2.VirtualMachineClass)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprv1alpha2.VirtualMachineClass{}, srcRaw)
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
	dst.Source = vmoprconversionmeta.SourceTypeMeta{
		APIVersion: c.SpokeGroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineClassConvertibleWrapper) ConvertFromHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineClass)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprvhub.VirtualMachineClass{}, srcRaw)
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Source.APIVersion != "" && src.Source.APIVersion != c.SpokeGroupVersionKind().GroupVersion().String() {
		errors.Errorf("src object originated from %s, it can't be converted to %s", src.Source.APIVersion, c.SpokeGroupVersionKind().GroupVersion().String())
	}

	dst, ok := dstRaw.(*vmoprv1alpha2.VirtualMachineClass)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprv1alpha2.VirtualMachineClass{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Hardware = vmoprv1alpha2.VirtualMachineClassHardware{
		Cpus:   src.Spec.Hardware.Cpus,
		Memory: src.Spec.Hardware.Memory,
	}

	return nil
}
