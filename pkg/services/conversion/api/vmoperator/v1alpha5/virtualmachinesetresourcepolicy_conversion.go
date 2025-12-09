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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineSetResourcePolicyConvertibleWrapper struct {
	*vmoprv1alpha5.VirtualMachineSetResourcePolicy
}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineSetResourcePolicyConvertibleWrapper{}

func (c *VirtualMachineSetResourcePolicyConvertibleWrapper) GroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineSetResourcePolicy")
}

func (c *VirtualMachineSetResourcePolicyConvertibleWrapper) Set(objRaw client.Object) {
	// FIXME: Chek what happens if cast fails
	c.VirtualMachineSetResourcePolicy = objRaw.(*vmoprv1alpha5.VirtualMachineSetResourcePolicy)
}

func (c *VirtualMachineSetResourcePolicyConvertibleWrapper) ConvertTo(dstRaw vmoprconversion.Hub) error {
	if c.VirtualMachineSetResourcePolicy == nil {
		return errors.New("method ConvertTo must be called after calling Set")
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineSetResourcePolicy)
	if !ok {
		return errors.New("dstRaw must be of type *vmoprvhub.VirtualMachineSetResourcePolicy")
	}

	src := c.VirtualMachineSetResourcePolicy
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.ClusterModuleGroups = src.Spec.ClusterModuleGroups
	dst.Spec.Folder = src.Spec.Folder
	dst.Spec.ResourcePool = vmoprvhub.ResourcePoolSpec{
		Name: src.Spec.ResourcePool.Name,
	}

	// The hub should keep track of the spoke version it was generated from.
	dst.Convertible = vmoprconversionmeta.TypeMetaConvertible{
		APIVersion: c.GroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineSetResourcePolicyConvertibleWrapper) ConvertFrom(srcRaw vmoprconversion.Hub) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineSetResourcePolicy)
	if !ok {
		errors.New("srcRaw must be of type *vmoprvhub.VirtualMachineSetResourcePolicy")
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Convertible.APIVersion != "" && src.Convertible.APIVersion != c.GroupVersionKind().GroupVersion().String() {
		errors.New("srcRaw must does not have the expected APIVersion") // FIXME:
	}

	dst := &vmoprv1alpha5.VirtualMachineSetResourcePolicy{}
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.ClusterModuleGroups = src.Spec.ClusterModuleGroups
	dst.Spec.Folder = src.Spec.Folder
	dst.Spec.ResourcePool = vmoprv1alpha5.ResourcePoolSpec{
		Name: src.Spec.ResourcePool.Name,
	}

	c.VirtualMachineSetResourcePolicy = dst
	return nil
}
