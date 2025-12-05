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

import (
	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/core/hub"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/util/conversion"
	utilmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/util/meta"
)

type VirtualMachineConverter struct {
	*vmoprv1alpha5.VirtualMachine
}

var _ conversion.Convertible = &VirtualMachineConverter{}

func (c *VirtualMachineConverter) GVK() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachine")
}

func (c *VirtualMachineConverter) Set(objRaw client.Object) {
	// FIXME: Chek what happens if cast fails
	c.VirtualMachine = objRaw.(*vmoprv1alpha5.VirtualMachine)
}

func (c *VirtualMachineConverter) NewList() client.ObjectList {
	return &vmoprv1alpha5.VirtualMachineList{}
}

func (c *VirtualMachineConverter) ConvertTo(dstRaw conversion.Hub) error {
	if c.VirtualMachine == nil {
		// TODO implement me
		panic("implement me")
	}

	dst := dstRaw.(*vmoprvhub.VirtualMachine)
	dst.TypeMeta = metav1.TypeMeta{
		Kind:       "VirtualMachine",
		APIVersion: vmoprvhub.GroupVersion.String(),
	}
	dst.Convertible = utilmeta.TypeMetaConvertible{
		APIVersion: vmoprv1alpha5.GroupVersion.String(),
	}
	dst.ObjectMeta = c.ObjectMeta

	return nil
}

func (c VirtualMachineConverter) ConvertFrom(srcRaw conversion.Hub) error {
	// TODO implement me
	panic("implement me")
}
