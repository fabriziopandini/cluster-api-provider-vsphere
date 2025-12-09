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

import "C"
import (
	"github.com/pkg/errors"
	vmoprv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineClassConvertibleWrapper struct {
	*vmoprv1alpha2.VirtualMachineClass
}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineClassConvertibleWrapper{}

func (c *VirtualMachineClassConvertibleWrapper) GroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha2.GroupVersion.WithKind("VirtualMachineClass")
}

func (c *VirtualMachineClassConvertibleWrapper) Set(objRaw client.Object) {
	// FIXME: Chek what happens if cast fails
	c.VirtualMachineClass = objRaw.(*vmoprv1alpha2.VirtualMachineClass)
}

func (c *VirtualMachineClassConvertibleWrapper) ConvertTo(dstRaw vmoprconversion.Hub) error {
	if c.VirtualMachineClass == nil {
		return errors.New("method ConvertTo must be called after calling Set")
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineClass)
	if !ok {
		return errors.New("dstRaw must be of type *vmoprvhub.VirtualMachineClass")
	}

	src := c.VirtualMachineClass
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

func (c *VirtualMachineClassConvertibleWrapper) ConvertFrom(srcRaw vmoprconversion.Hub) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineClass)
	if !ok {
		errors.New("srcRaw must be of type *vmoprvhub.VirtualMachineClass")
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Convertible.APIVersion != "" && src.Convertible.APIVersion != c.GroupVersionKind().GroupVersion().String() {
		errors.New("srcRaw must does not have the expected APIVersion") // FIXME:
	}

	dst := &vmoprv1alpha2.VirtualMachineClass{}
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Hardware = vmoprv1alpha2.VirtualMachineClassHardware{
		Cpus:   src.Spec.Hardware.Cpus,
		Memory: src.Spec.Hardware.Memory,
	}

	c.VirtualMachineClass = dst
	return nil
}
