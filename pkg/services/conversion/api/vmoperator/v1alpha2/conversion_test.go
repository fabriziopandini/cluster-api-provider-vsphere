//go:build !race

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

import (
	"testing"

	vmoprv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	vmoprconversion "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion"
	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for VirtualMachine", vmoprconversion.RoundTripTest(vmoprconversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachine{},
		Spoke:        &vmoprv1alpha2.VirtualMachine{},
		SpokeWrapper: &VirtualMachineConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineService", vmoprconversion.RoundTripTest(vmoprconversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineService{},
		Spoke:        &vmoprv1alpha2.VirtualMachineService{},
		SpokeWrapper: &VirtualMachineServiceConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineSetResourcePolicy", vmoprconversion.RoundTripTest(vmoprconversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineSetResourcePolicy{},
		Spoke:        &vmoprv1alpha2.VirtualMachineSetResourcePolicy{},
		SpokeWrapper: &VirtualMachineSetResourcePolicyConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineClass", vmoprconversion.RoundTripTest(vmoprconversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineClass{},
		Spoke:        &vmoprv1alpha2.VirtualMachineClass{},
		SpokeWrapper: &VirtualMachineClassConvertibleWrapper{},
	}))
	t.Run("for VirtualMachineImage", vmoprconversion.RoundTripTest(vmoprconversion.RoundTripTestInput{
		Hub:          &vmoprvhub.VirtualMachineImage{},
		Spoke:        &vmoprv1alpha2.VirtualMachineImage{},
		SpokeWrapper: &VirtualMachineImageConvertibleWrapper{},
	}))
}
