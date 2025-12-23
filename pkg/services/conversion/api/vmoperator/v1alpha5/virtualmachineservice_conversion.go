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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineServiceConvertibleWrapper struct{}

var _ vmoprconversion.ConvertibleWrapper = &VirtualMachineServiceConvertibleWrapper{}

func (c *VirtualMachineServiceConvertibleWrapper) SpokeGroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineService")
}

func (c *VirtualMachineServiceConvertibleWrapper) ConvertToHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprv1alpha5.VirtualMachineService)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineService{}, srcRaw)
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineService)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprvhub.VirtualMachineService{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.Ports != nil {
		dst.Spec.Ports = []vmoprvhub.VirtualMachineServicePort{}
		for _, port := range src.Spec.Ports {
			dst.Spec.Ports = append(dst.Spec.Ports, vmoprvhub.VirtualMachineServicePort{
				Name:       port.Name,
				Protocol:   port.Protocol,
				Port:       port.Port,
				TargetPort: port.TargetPort,
			})
		}
	}
	dst.Spec.Selector = src.Spec.Selector
	dst.Spec.Type = vmoprvhub.VirtualMachineServiceType(src.Spec.Type)

	if src.Status.LoadBalancer.Ingress != nil {
		dst.Status.LoadBalancer.Ingress = []vmoprvhub.LoadBalancerIngress{}
		for _, ingress := range src.Status.LoadBalancer.Ingress {
			dst.Status.LoadBalancer.Ingress = append(dst.Status.LoadBalancer.Ingress, vmoprvhub.LoadBalancerIngress{
				IP: ingress.IP,
			})
		}
	}

	// The hub should keep track of the spoke version it was generated from.
	dst.Source = vmoprconversionmeta.SourceTypeMeta{
		APIVersion: c.SpokeGroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineServiceConvertibleWrapper) ConvertFromHub(srcRaw runtime.Object, dstRaw runtime.Object) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineService)
	if !ok {
		return errors.Errorf("src object must be of type %T, got %T", &vmoprvhub.VirtualMachineService{}, srcRaw)
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Source.APIVersion != "" && src.Source.APIVersion != c.SpokeGroupVersionKind().GroupVersion().String() {
		errors.Errorf("src object originated from %s, it can't be converted to %s", src.Source.APIVersion, c.SpokeGroupVersionKind().GroupVersion().String())
	}

	dst, ok := dstRaw.(*vmoprv1alpha5.VirtualMachineService)
	if !ok {
		return errors.Errorf("dst object must be of type %T, got %T", &vmoprv1alpha5.VirtualMachineService{}, dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if src.Spec.Ports != nil {
		dst.Spec.Ports = []vmoprv1alpha5.VirtualMachineServicePort{}
		for _, port := range src.Spec.Ports {
			dst.Spec.Ports = append(dst.Spec.Ports, vmoprv1alpha5.VirtualMachineServicePort{
				Name:       port.Name,
				Protocol:   port.Protocol,
				Port:       port.Port,
				TargetPort: port.TargetPort,
			})
		}
	}
	dst.Spec.Selector = src.Spec.Selector
	dst.Spec.Type = vmoprv1alpha5.VirtualMachineServiceType(src.Spec.Type)

	if src.Status.LoadBalancer.Ingress != nil {
		dst.Status.LoadBalancer.Ingress = []vmoprv1alpha5.LoadBalancerIngress{}
		for _, ingress := range src.Status.LoadBalancer.Ingress {
			dst.Status.LoadBalancer.Ingress = append(dst.Status.LoadBalancer.Ingress, vmoprv1alpha5.LoadBalancerIngress{
				IP: ingress.IP,
			})
		}
	}

	return nil
}
