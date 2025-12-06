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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	utilmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

type VirtualMachineConvertibleWrapper struct {
	*vmoprv1alpha5.VirtualMachine
}

var _ conversion.ConvertibleWrapper = &VirtualMachineConvertibleWrapper{}

func (c *VirtualMachineConvertibleWrapper) GroupVersionKind() schema.GroupVersionKind {
	return vmoprv1alpha5.GroupVersion.WithKind("VirtualMachine")
}

func (c *VirtualMachineConvertibleWrapper) Set(objRaw client.Object) {
	// FIXME: Chek what happens if cast fails
	c.VirtualMachine = objRaw.(*vmoprv1alpha5.VirtualMachine)
}

func (c *VirtualMachineConvertibleWrapper) ConvertTo(dstRaw conversion.Hub) error {
	if c.VirtualMachine == nil {
		return errors.New("method ConvertTo must be called after calling Set")
	}

	dst, ok := dstRaw.(*vmoprvhub.VirtualMachine)
	if !ok {
		return errors.New("dstRaw must be of type *vmoprvhub.VirtualMachine")
	}

	src := c.VirtualMachine
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Bootstrap != nil {
		dst.Spec.Bootstrap = &vmoprvhub.VirtualMachineBootstrapSpec{}
		if src.Spec.Bootstrap.CloudInit != nil {
			dst.Spec.Bootstrap.CloudInit = &vmoprvhub.VirtualMachineBootstrapCloudInitSpec{}
			if src.Spec.Bootstrap.CloudInit.RawCloudConfig != nil {
				dst.Spec.Bootstrap.CloudInit.RawCloudConfig = &vmoprvhub.SecretKeySelector{
					Name: src.Spec.Bootstrap.CloudInit.RawCloudConfig.Name,
					Key:  src.Spec.Bootstrap.CloudInit.RawCloudConfig.Key,
				}
			}
		}
	}
	dst.Spec.ClassName = src.Spec.ClassName
	dst.Spec.ImageName = src.Spec.ImageName
	if src.Spec.Network != nil {
		dst.Spec.Network = &vmoprvhub.VirtualMachineNetworkSpec{}
		if src.Spec.Network.Interfaces != nil {
			dst.Spec.Network.Interfaces = []vmoprvhub.VirtualMachineNetworkInterfaceSpec{}
			for _, iface := range src.Spec.Network.Interfaces {
				d := vmoprvhub.VirtualMachineNetworkInterfaceSpec{}
				d.Gateway4 = iface.Gateway4
				d.Gateway6 = iface.Gateway6
				if iface.MTU != nil {
					d.MTU = ptr.To(*iface.MTU)
				}
				if iface.Network != nil {
					d.Network = &vmoprvhub.PartialObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       iface.Network.Kind,
							APIVersion: iface.Network.APIVersion,
						},
						Name: iface.Network.Name,
					}
				}
				d.Name = iface.Name
				if iface.Routes != nil {
					d.Routes = []vmoprvhub.VirtualMachineNetworkRouteSpec{}
					for _, route := range iface.Routes {
						d.Routes = append(d.Routes, vmoprvhub.VirtualMachineNetworkRouteSpec{
							To:  route.To,
							Via: route.Via,
						})
					}
				}
				dst.Spec.Network.Interfaces = append(dst.Spec.Network.Interfaces, d)
			}
		}
	}
	dst.Spec.MinHardwareVersion = src.Spec.MinHardwareVersion
	dst.Spec.PowerOffMode = vmoprvhub.VirtualMachinePowerOpMode(src.Spec.PowerOffMode)
	dst.Spec.PowerState = vmoprvhub.VirtualMachinePowerState(src.Spec.PowerState)
	if src.Spec.ReadinessProbe != nil {
		dst.Spec.ReadinessProbe = &vmoprvhub.VirtualMachineReadinessProbeSpec{}
		if src.Spec.ReadinessProbe.TCPSocket != nil {
			dst.Spec.ReadinessProbe.TCPSocket = &vmoprvhub.TCPSocketAction{
				Port: src.Spec.ReadinessProbe.TCPSocket.Port,
				Host: src.Spec.ReadinessProbe.TCPSocket.Host,
			}
		}
	}
	if src.Spec.Reserved != nil {
		dst.Spec.Reserved = &vmoprvhub.VirtualMachineReservedSpec{
			ResourcePolicyName: src.Spec.Reserved.ResourcePolicyName,
		}
	}
	dst.Spec.StorageClass = src.Spec.StorageClass
	if src.Spec.Volumes != nil {
		dst.Spec.Volumes = []vmoprvhub.VirtualMachineVolume{}
		for _, volume := range src.Spec.Volumes {
			v := vmoprvhub.VirtualMachineVolume{}
			v.Name = volume.Name
			if volume.PersistentVolumeClaim != nil {
				v.PersistentVolumeClaim = &vmoprvhub.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: volume.PersistentVolumeClaim.ClaimName,
						ReadOnly:  volume.PersistentVolumeClaim.ReadOnly,
					},
				}
			}
			dst.Spec.Volumes = append(dst.Spec.Volumes, v)
		}
	}

	dst.Status.BiosUUID = src.Status.BiosUUID
	// FIXME: dst.Status.Host =
	if src.Status.Network != nil {
		dst.Status.Network = &vmoprvhub.VirtualMachineNetworkStatus{}
		if src.Status.Network.Interfaces != nil {
			dst.Status.Network.Interfaces = []vmoprvhub.VirtualMachineNetworkInterfaceStatus{}
			for _, iface := range src.Status.Network.Interfaces {
				d := vmoprvhub.VirtualMachineNetworkInterfaceStatus{}
				d.DeviceKey = iface.DeviceKey
				if iface.DNS != nil {
					d.DNS = &vmoprvhub.VirtualMachineNetworkDNSStatus{
						DHCP:          iface.DNS.DHCP,
						HostName:      iface.DNS.HostName,
						DomainName:    iface.DNS.DomainName,
						Nameservers:   iface.DNS.Nameservers,
						SearchDomains: iface.DNS.SearchDomains,
					}
				}
				if iface.IP != nil {
				}
				d.Name = iface.Name
				dst.Status.Network.Interfaces = append(dst.Status.Network.Interfaces, d)
			}
		}
		dst.Status.Network.PrimaryIP4 = src.Status.Network.PrimaryIP4
		dst.Status.Network.PrimaryIP6 = src.Status.Network.PrimaryIP6
	}
	dst.Status.PowerState = vmoprvhub.VirtualMachinePowerState(src.Status.PowerState)

	// The hub should keep track of the spoke version it was generated from.
	dst.Convertible = utilmeta.TypeMetaConvertible{
		APIVersion: c.GroupVersionKind().GroupVersion().String(),
	}
	return nil
}

func (c *VirtualMachineConvertibleWrapper) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*vmoprvhub.VirtualMachine)
	if !ok {
		errors.New("srcRaw must be of type *vmoprvhub.VirtualMachine")
	}

	// Check if the hub is new or it was generated from the spoke version we are converting to.
	if src.Convertible.APIVersion != "" && src.Convertible.APIVersion != c.GroupVersionKind().GroupVersion().String() {
		errors.New("srcRaw must does not have the expected APIVersion") // FIXME:
	}

	dst := &vmoprv1alpha5.VirtualMachine{}
	dst.ObjectMeta = src.ObjectMeta
	if src.Spec.Bootstrap != nil {
		dst.Spec.Bootstrap = &vmoprv1alpha5.VirtualMachineBootstrapSpec{}
		if src.Spec.Bootstrap.CloudInit != nil {
			dst.Spec.Bootstrap.CloudInit = &vmoprv1alpha5.VirtualMachineBootstrapCloudInitSpec{}
			if src.Spec.Bootstrap.CloudInit.RawCloudConfig != nil {
				dst.Spec.Bootstrap.CloudInit.RawCloudConfig = &vmoprv1alpha5common.SecretKeySelector{
					Name: src.Spec.Bootstrap.CloudInit.RawCloudConfig.Name,
					Key:  src.Spec.Bootstrap.CloudInit.RawCloudConfig.Key,
				}
			}
		}
	}
	dst.Spec.ClassName = src.Spec.ClassName
	dst.Spec.ImageName = src.Spec.ImageName
	if src.Spec.Network != nil {
		dst.Spec.Network = &vmoprv1alpha5.VirtualMachineNetworkSpec{}
		for _, iface := range src.Spec.Network.Interfaces {
			d := vmoprv1alpha5.VirtualMachineNetworkInterfaceSpec{}
			d.Gateway4 = iface.Gateway4
			d.Gateway6 = iface.Gateway6
			if iface.MTU != nil {
				d.MTU = ptr.To(*iface.MTU)
			}
			if iface.Network != nil {
				d.Network = &vmoprv1alpha5common.PartialObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       iface.Network.Kind,
						APIVersion: iface.Network.APIVersion,
					},
					Name: iface.Network.Name,
				}
			}
			d.Name = iface.Name
			if iface.Routes != nil {
				d.Routes = []vmoprv1alpha5.VirtualMachineNetworkRouteSpec{}
				for _, route := range iface.Routes {
					d.Routes = append(d.Routes, vmoprv1alpha5.VirtualMachineNetworkRouteSpec{
						To:  route.To,
						Via: route.Via,
					})
				}
			}
			dst.Spec.Network.Interfaces = append(dst.Spec.Network.Interfaces, d)
		}
	}
	dst.Spec.MinHardwareVersion = src.Spec.MinHardwareVersion
	dst.Spec.PowerOffMode = vmoprv1alpha5.VirtualMachinePowerOpMode(src.Spec.PowerOffMode)
	dst.Spec.PowerState = vmoprv1alpha5.VirtualMachinePowerState(src.Spec.PowerState)
	if src.Spec.ReadinessProbe != nil {
		dst.Spec.ReadinessProbe = &vmoprv1alpha5.VirtualMachineReadinessProbeSpec{}
		if src.Spec.ReadinessProbe.TCPSocket != nil {
			dst.Spec.ReadinessProbe.TCPSocket = &vmoprv1alpha5.TCPSocketAction{
				Port: src.Spec.ReadinessProbe.TCPSocket.Port,
				Host: src.Spec.ReadinessProbe.TCPSocket.Host,
			}
		}
	}
	if src.Spec.Reserved != nil {
		dst.Spec.Reserved = &vmoprv1alpha5.VirtualMachineReservedSpec{
			ResourcePolicyName: src.Spec.Reserved.ResourcePolicyName,
		}
	}
	dst.Spec.StorageClass = src.Spec.StorageClass
	if src.Spec.Volumes != nil {
		for _, volume := range src.Spec.Volumes {
			dst.Spec.Volumes = []vmoprv1alpha5.VirtualMachineVolume{}
			v := vmoprv1alpha5.VirtualMachineVolume{}
			v.Name = volume.Name
			if volume.PersistentVolumeClaim != nil {
				v.PersistentVolumeClaim = &vmoprv1alpha5.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: volume.PersistentVolumeClaim.ClaimName,
						ReadOnly:  volume.PersistentVolumeClaim.ReadOnly,
					},
				}
			}
			dst.Spec.Volumes = append(dst.Spec.Volumes, v)
		}
	}
	dst.Status.BiosUUID = src.Status.BiosUUID
	// FIXME: dst.Status.Host =
	if src.Status.Network != nil {
		dst.Status.Network = &vmoprv1alpha5.VirtualMachineNetworkStatus{}
		if src.Status.Network.Interfaces != nil {
			dst.Status.Network.Interfaces = []vmoprv1alpha5.VirtualMachineNetworkInterfaceStatus{}
			for _, iface := range src.Status.Network.Interfaces {
				d := vmoprv1alpha5.VirtualMachineNetworkInterfaceStatus{}
				d.DeviceKey = iface.DeviceKey
				if iface.DNS != nil {
					d.DNS = &vmoprv1alpha5.VirtualMachineNetworkDNSStatus{
						DHCP:          iface.DNS.DHCP,
						HostName:      iface.DNS.HostName,
						DomainName:    iface.DNS.DomainName,
						Nameservers:   iface.DNS.Nameservers,
						SearchDomains: iface.DNS.SearchDomains,
					}
				}
				if iface.IP != nil {
				}
				d.Name = iface.Name
				dst.Status.Network.Interfaces = append(dst.Status.Network.Interfaces, d)
			}
		}
		dst.Status.Network.PrimaryIP4 = src.Status.Network.PrimaryIP4
		dst.Status.Network.PrimaryIP6 = src.Status.Network.PrimaryIP6
	}
	dst.Status.PowerState = vmoprv1alpha5.VirtualMachinePowerState(src.Status.PowerState)

	c.VirtualMachine = dst
	return nil
}
