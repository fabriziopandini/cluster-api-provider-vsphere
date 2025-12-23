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
	"github.com/pkg/errors"
	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineGroupConvertibleWrapper struct{}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineGroupConvertibleWrapper{}

func (c *VirtualMachineGroupConvertibleWrapper) SpokeGroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineGroup")
}

func (c *VirtualMachineGroupConvertibleWrapper) ConvertToHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprv1alpha5.VirtualMachineGroup)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineGroup{}, srcRaw)
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineGroup)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprvhub.VirtualMachineGroup{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.BootOrder != nil {
		dst.Spec.BootOrder = []vmoprvhub.VirtualMachineGroupBootOrderGroup{}
		for _, bootOrderGroup := range src.Spec.BootOrder {
			bg := vmoprvhub.VirtualMachineGroupBootOrderGroup{}
			if bootOrderGroup.Members != nil {
				bg.Members = []vmoprvhub.GroupMember{}
				for _, member := range bootOrderGroup.Members {
					bg.Members = append(bg.Members, vmoprvhub.GroupMember{
						Name: member.Name,
						Kind: member.Kind,
					})
				}
			}
			dst.Spec.BootOrder = append(dst.Spec.BootOrder, bg)
		}
	}
	if src.Status.Members != nil {
		dst.Status.Members = []vmoprvhub.VirtualMachineGroupMemberStatus{}
		for _, member := range src.Status.Members {
			m := vmoprvhub.VirtualMachineGroupMemberStatus{
				Name: member.Name,
			}
			if member.Placement != nil {
				m.Placement = &vmoprvhub.VirtualMachinePlacementStatus{
					Zone: member.Placement.Zone,
				}
			}
			if member.Conditions != nil {
				m.Conditions = []metav1.Condition{}
				for _, condition := range member.Conditions {
					m.Conditions = append(m.Conditions, condition)
				}
			}
			dst.Status.Members = append(dst.Status.Members, m)
		}
	}

	// The hub should keep track of the spoke version it was generated from.
	dst.Source = vmoprconversionmeta.SourceTypeMeta{
		APIVersion: c.SpokeGroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineGroupConvertibleWrapper) ConvertFromHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineGroup)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprvhub.VirtualMachineGroup{}, srcRaw)
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Source.APIVersion != "" && src.Source.APIVersion != c.SpokeGroupVersionKind().GroupVersion().String() {
		errors.Errorf("src object originated from %s, it can't be converted to %s", src.Source.APIVersion, c.SpokeGroupVersionKind().GroupVersion().String())
	}

	dst, ok := dstRaw.(*vmoprv1alpha5.VirtualMachineGroup)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineGroup{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.BootOrder != nil {
		dst.Spec.BootOrder = []vmoprv1alpha5.VirtualMachineGroupBootOrderGroup{}
		for _, bootOrderGroup := range src.Spec.BootOrder {
			bg := vmoprv1alpha5.VirtualMachineGroupBootOrderGroup{}
			if bootOrderGroup.Members != nil {
				bg.Members = []vmoprv1alpha5.GroupMember{}
				for _, member := range bootOrderGroup.Members {
					bg.Members = append(bg.Members, vmoprv1alpha5.GroupMember{
						Name: member.Name,
						Kind: member.Kind,
					})
				}
			}
			dst.Spec.BootOrder = append(dst.Spec.BootOrder, bg)
		}
	}
	if src.Status.Members != nil {
		dst.Status.Members = []vmoprv1alpha5.VirtualMachineGroupMemberStatus{}
		for _, member := range src.Status.Members {
			m := vmoprv1alpha5.VirtualMachineGroupMemberStatus{
				Name: member.Name,
			}
			if member.Placement != nil {
				m.Placement = &vmoprv1alpha5.VirtualMachinePlacementStatus{
					Zone: member.Placement.Zone,
				}
			}
			if member.Conditions != nil {
				m.Conditions = []metav1.Condition{}
				for _, condition := range member.Conditions {
					m.Conditions = append(m.Conditions, condition)
				}
			}
			dst.Status.Members = append(dst.Status.Members, m)
		}
	}

	return nil
}
