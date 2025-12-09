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

package hub

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

// ResourcePoolSpec defines a Logical Grouping of workloads that share resource
// policies.
type ResourcePoolSpec struct {
	// +optional

	// Name describes the name of the ResourcePool grouping.
	Name string `json:"name,omitempty"`

	/*
		// +optional

		// Reservations describes the guaranteed resources reserved for the
		// ResourcePool.
		Reservations VirtualMachineResourceSpec `json:"reservations,omitempty"`

		// +optional

		// Limits describes the limit to resources available to the ResourcePool.
		Limits VirtualMachineResourceSpec `json:"limits,omitempty"`
	*/
}

// VirtualMachineSetResourcePolicySpec defines the desired state of
// VirtualMachineSetResourcePolicy.
type VirtualMachineSetResourcePolicySpec struct {
	ResourcePool        ResourcePoolSpec `json:"resourcePool,omitempty"`
	Folder              string           `json:"folder,omitempty"`
	ClusterModuleGroups []string         `json:"clusterModuleGroups,omitempty"`
}

// VirtualMachineSetResourcePolicyStatus defines the observed state of
// VirtualMachineSetResourcePolicy.
type VirtualMachineSetResourcePolicyStatus struct {
	/*
		ResourcePools  []ResourcePoolStatus         `json:"resourcePools,omitempty"`
		ClusterModules []VSphereClusterModuleStatus `json:"clustermodules,omitempty"`
	*/
}

/*
// ResourcePoolStatus describes the observed state of a vSphere child
// resource pool created for the Spec.ResourcePool.Name.
type ResourcePoolStatus struct {
	ClusterMoID           string `json:"clusterMoID"`
	ChildResourcePoolMoID string `json:"childResourcePoolMoID"`
}

// VSphereClusterModuleStatus describes the observed state of a vSphere
// cluster module.
type VSphereClusterModuleStatus struct {
	GroupName   string `json:"groupName"`
	ModuleUuid  string `json:"moduleUUID"` //nolint:revive
	ClusterMoID string `json:"clusterMoID"`
}
*/

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API.
type VirtualMachineSetResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSetResourcePolicySpec   `json:"spec,omitempty"`
	Status VirtualMachineSetResourcePolicyStatus `json:"status,omitempty"`

	// FIXME: think about name
	Convertible vmoprconversionmeta.TypeMetaConvertible `json:"convertible,omitempty"`
}

func (p *VirtualMachineSetResourcePolicy) NamespacedName() string {
	return p.Namespace + "/" + p.Name
}

// +kubebuilder:object:root=true

// VirtualMachineSetResourcePolicyList contains a list of VirtualMachineSetResourcePolicy.
type VirtualMachineSetResourcePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineSetResourcePolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &VirtualMachineSetResourcePolicy{}, &VirtualMachineSetResourcePolicyList{})
}

var _ vmoprconversion.Hub = &VirtualMachineSetResourcePolicy{}

func (p *VirtualMachineSetResourcePolicy) Hub() {}

func (p *VirtualMachineSetResourcePolicy) SetConvertibleAPIVersion(v string) {
	p.Convertible.APIVersion = v
}

func (p *VirtualMachineSetResourcePolicy) GetConvertibleAPIVersion() string {
	return p.Convertible.APIVersion
}
