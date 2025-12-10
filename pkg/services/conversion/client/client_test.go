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

package client

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	vmoprv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmoprv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmoprvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/api/vmoperator/hub"
	vmoprconversionmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/meta"
)

var (
	ctx    = context.TODO()
	scheme = runtime.NewScheme()
)

func init() {
	_ = vmoprvhub.AddToScheme(scheme)
	_ = vmoprv1alpha2.AddToScheme(scheme)
	_ = vmoprv1alpha5.AddToScheme(scheme)
}

func Test_conversionClient_Get(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		spokeObj         client.Object
		wantHubObj       client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			spokeObj: &vmoprv1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantHubObj: &vmoprvhub.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
				Convertible: vmoprconversionmeta.TypeMetaConvertible{
					APIVersion: vmoprv1alpha2.GroupVersion.String(),
				},
			},
			wantErr: false,
		},
		{
			name:             "Get VirtualMachine when preferred version is v1alpha5",
			preferredVersion: "v1alpha5",
			spokeObj: &vmoprv1alpha5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantHubObj: &vmoprvhub.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
				Convertible: vmoprconversionmeta.TypeMetaConvertible{
					APIVersion: vmoprv1alpha5.GroupVersion.String(),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := conversionClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.spokeObj).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			hubObj := &vmoprvhub.VirtualMachine{}
			err := c.Get(ctx, client.ObjectKeyFromObject(tt.spokeObj), hubObj)
			if (err != nil) != tt.wantErr {
				g.Fail(fmt.Sprintf("Get() error = %v, wantErr %v", err, tt.wantErr))
			}

			tt.wantHubObj.SetResourceVersion(hubObj.GetResourceVersion())
			g.Expect(hubObj).To(Equal(tt.wantHubObj))
		})
	}
}

func Test_conversionClient_List(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		spokeObjs        []client.Object
		wantHubObjs      []client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			spokeObjs: []client.Object{
				&vmoprv1alpha2.VirtualMachine{
					TypeMeta: metav1.TypeMeta{
						Kind:       "VirtualMachine",
						APIVersion: vmoprv1alpha2.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: "test-ns",
					},
				},
			},
			wantHubObjs: []client.Object{
				&vmoprvhub.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: "test-ns",
					},
					Convertible: vmoprconversionmeta.TypeMetaConvertible{
						APIVersion: vmoprv1alpha2.GroupVersion.String(),
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := conversionClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.spokeObjs...).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			hubObjs := &vmoprvhub.VirtualMachineList{}
			err := c.List(ctx, hubObjs)
			if (err != nil) != tt.wantErr {
				g.Fail(fmt.Sprintf("Get() error = %v, wantErr %v", err, tt.wantErr))
			}

			g.Expect(len(hubObjs.Items)).To(Equal(len(tt.wantHubObjs)))
			for i, wantHubObj := range tt.wantHubObjs {
				wantHubObj.SetResourceVersion(hubObjs.Items[i].GetResourceVersion())
				g.Expect(&hubObjs.Items[i]).To(Equal(wantHubObj))
			}
		})
	}
}

func Test_conversionClient_Create(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		hubObj           client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			hubObj: &vmoprvhub.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
				Spec: vmoprvhub.VirtualMachineSpec{
					ClassName: "test-class",
				},
				Convertible: vmoprconversionmeta.TypeMetaConvertible{
					APIVersion: vmoprv1alpha2.GroupVersion.String(),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := conversionClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			hubOriginal := tt.hubObj.DeepCopyObject().(client.Object)

			err := c.Create(ctx, tt.hubObj)
			if (err != nil) != tt.wantErr {
				g.Fail(fmt.Sprintf("Get() error = %v, wantErr %v", err, tt.wantErr))
			}

			hubObj := &vmoprvhub.VirtualMachine{}
			err = c.Get(ctx, client.ObjectKeyFromObject(tt.hubObj), hubObj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(hubObj.GetResourceVersion()).ToNot(BeEmpty())
			hubOriginal.SetResourceVersion(hubObj.GetResourceVersion())
			g.Expect(hubObj).To(Equal(hubOriginal))
		})
	}
}

func Test_conversionClient_Patch(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		hubObj           client.Object
		modifyFunc       func(client.Object) client.Object
		wantSpokeObj     client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			hubObj: &vmoprvhub.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
				Spec: vmoprvhub.VirtualMachineSpec{
					ClassName: "test-class",
				},
				Convertible: vmoprconversionmeta.TypeMetaConvertible{
					APIVersion: vmoprv1alpha2.GroupVersion.String(),
				},
			},
			modifyFunc: func(o client.Object) client.Object {
				vm := o.(*vmoprvhub.VirtualMachine)
				vm.Spec.ClassName = "another-class"
				return vm
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := conversionClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			err := c.Create(ctx, tt.hubObj)
			hubObjModified := tt.modifyFunc(tt.hubObj.(*vmoprvhub.VirtualMachine))

			err = c.Patch(ctx, hubObjModified, MergeFrom(tt.hubObj))
			if (err != nil) != tt.wantErr {
				g.Fail(fmt.Sprintf("Get() error = %v, wantErr %v", err, tt.wantErr))
			}

			hubObj := &vmoprvhub.VirtualMachine{}
			err = c.Get(ctx, client.ObjectKeyFromObject(hubObjModified), hubObj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(hubObj).To(Equal(hubObjModified))
		})
	}
}
