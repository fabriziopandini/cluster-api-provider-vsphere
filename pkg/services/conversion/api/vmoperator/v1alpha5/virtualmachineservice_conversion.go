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

	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	utilmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineServiceConvertibleWrapper struct {
	*vmoprv1alpha5.VirtualMachineService
}

var _ conversion.ConvertibleWrapper = &VirtualMachineServiceConvertibleWrapper{}

func (c *VirtualMachineServiceConvertibleWrapper) GroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachineService")
}

func (c *VirtualMachineServiceConvertibleWrapper) Set(objRaw client.Object) {
	// FIXME: Chek what happens if cast fails
	c.VirtualMachineService = objRaw.(*vmoprv1alpha5.VirtualMachineService)
}

func (c *VirtualMachineServiceConvertibleWrapper) ConvertTo(dstRaw conversion.Hub) error {
	if c.VirtualMachineService == nil {
		return errors.New("method ConvertTo must be called after calling Set")
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachineService)
	if !ok {
		return errors.New("dstRaw must be of type *vmoprvhub.VirtualMachineService")
	}

	src := c.VirtualMachineService
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
	dst.Convertible = utilmeta.TypeMetaConvertible{
		APIVersion: c.GroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineServiceConvertibleWrapper) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachineService)
	if !ok {
		errors.New("srcRaw must be of type *vmoprvhub.VirtualMachineService")
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Convertible.APIVersion != "" && src.Convertible.APIVersion != c.GroupVersionKind().GroupVersion().String() {
		errors.New("srcRaw must does not have the expected APIVersion") // FIXME:
	}

	dst := &vmoprv1alpha5.VirtualMachineService{}
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

	c.VirtualMachineService = dst
	return nil
}
