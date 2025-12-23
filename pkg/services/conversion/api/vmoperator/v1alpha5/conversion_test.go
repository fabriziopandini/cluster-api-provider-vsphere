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
	"testing"

	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"

	conversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for VirtualMachine", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachine{},
		Spoke:        &vmoprv1alpha5.VirtualMachine{},
		SpokeWrapper: &VirtualMachineConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineClass", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineClass{},
		Spoke:        &vmoprv1alpha5.VirtualMachineClass{},
		SpokeWrapper: &VirtualMachineClassConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineGroup", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineGroup{},
		Spoke:        &vmoprv1alpha5.VirtualMachineGroup{},
		SpokeWrapper: &VirtualMachineGroupConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineImage", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineImage{},
		Spoke:        &vmoprv1alpha5.VirtualMachineImage{},
		SpokeWrapper: &VirtualMachineImageConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineService", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineService{},
		Spoke:        &vmoprv1alpha5.VirtualMachineService{},
		SpokeWrapper: &VirtualMachineServiceConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineSetResourcePolicy", conversion.RoundTripTest(conversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineSetResourcePolicy{},
		Spoke:        &vmoprv1alpha5.VirtualMachineSetResourcePolicy{},
		SpokeWrapper: &VirtualMachineSetResourcePolicyConvertibleWrapper{},
	}))
}
