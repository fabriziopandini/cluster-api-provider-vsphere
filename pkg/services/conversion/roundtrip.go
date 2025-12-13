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

package conversion

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/randfill"

	conversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

// RoundTripTestInput contains input parameters
// for the RoundTripTest function.
type RoundTripTestInput struct {
	Scheme *runtime.Scheme

	Hub              Hub
	HubAfterMutation func(Hub)

	SpokeWrapper               ConvertibleWrapper
	Spoke                      client.Object
	SpokeAfterMutation         func(convertible ConvertibleWrapper)
	SkipSpokeAnnotationCleanup bool

	FuzzerFuncs []any
}

// RoundTripTest returns a new testing function to be used in tests to make sure conversions between
// the Hub version of an object and an the corresponding Spoke version aren't lossy.
func RoundTripTest(input RoundTripTestInput) func(*testing.T) {
	if input.Scheme == nil {
		input.Scheme = scheme.Scheme
	}

	return func(t *testing.T) {
		t.Helper()
		t.Run("hub-spoke-hub", func(t *testing.T) {
			fuzzer := conversionutil.GetFuzzer(input.Scheme, func(_ runtimeserializer.CodecFactory) []any {
				return append(input.FuzzerFuncs, func(_ *conversionmeta.SourceTypeMeta, _ randfill.Continue) {
					// Ensure SourceTypeMeta is not set by the fuzzer.
				})
			})

			for range 10000 {
				// Create the hub and fuzz it
				hubBefore := input.Hub.DeepCopyObject().(Hub)
				fuzzer.Fill(hubBefore)

				// First convert hub to spoke
				spokeWrapper := input.SpokeWrapper
				spoke := input.Spoke.DeepCopyObject()
				if err := spokeWrapper.ConvertFromHub(hubBefore, spoke); err != nil {
					t.Fatalf("error calling ConvertFrom: %v", err)
				}

				// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
				hubAfter := input.Hub.DeepCopyObject().(Hub)
				if err := spokeWrapper.ConvertToHub(spoke, hubAfter); err != nil {
					t.Fatalf("error calling ConvertTo: %v", err)
				}
				if hubAfter.GetSourceAPIVersion() != spokeWrapper.SpokeGroupVersionKind().GroupVersion().String() {
					t.Fatal("ConvertTo is expected to set Convertible.APIVersion")
				}
				hubAfter.SetSourceAPIVersion("")

				if input.HubAfterMutation != nil {
					input.HubAfterMutation(hubAfter)
				}

				if !apiequality.Semantic.DeepEqual(hubBefore, hubAfter) {
					diff := cmp.Diff(hubBefore, hubAfter)
					t.Fatal(diff)
				}
			}
		})
	}
}
