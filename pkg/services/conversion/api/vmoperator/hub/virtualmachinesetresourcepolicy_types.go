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

	conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	conversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

// ResourcePoolSpec defines a Logical Grouping of workloads that share resource
// policies.
type ResourcePoolSpec struct {
	// +optional

	// Name describes the name of the ResourcePool grouping.
	Name string `json:"name,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API.
type VirtualMachineSetResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSetResourcePolicySpec   `json:"spec,omitempty"`
	Status VirtualMachineSetResourcePolicyStatus `json:"status,omitempty"`

	Source conversionmeta.SourceTypeMeta `json:"source,omitempty"`
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

var _ conversion.Hub = &VirtualMachineSetResourcePolicy{}

// SetSourceAPIVersion set the API version this object is converted from.
func (p *VirtualMachineSetResourcePolicy) SetSourceAPIVersion(v string) {
	p.Source.APIVersion = v
}

// GetSourceAPIVersion grt the API version this object is converted from.
func (p *VirtualMachineSetResourcePolicy) GetSourceAPIVersion() string {
	return p.Source.APIVersion
}
