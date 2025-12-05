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

package util

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	vmopv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1alpha5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopvhub "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/core/hub"
	utilmeta "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/vmoperator/api/util/meta"
)

var (
	ctx    = context.TODO()
	scheme = runtime.NewScheme()
)

func init() {
	_ = vmopvhub.AddToScheme(scheme)
	_ = vmopv1alpha2.AddToScheme(scheme)
	_ = vmopv1alpha5.AddToScheme(scheme)
}

func Test_versionAwareClient_Get(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		vmopObj          client.Object
		wantHubObj       client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			vmopObj: &vmopv1alpha2.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       "VirtualMachine",
					APIVersion: vmopv1alpha2.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantHubObj: &vmopvhub.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       "VirtualMachine",
					APIVersion: vmopvhub.GroupVersion.String(),
				},
				Convertible: utilmeta.TypeMetaConvertible{
					APIVersion: vmopv1alpha2.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
		},
		{
			name:             "Get VirtualMachine when preferred version is v1alpha5",
			preferredVersion: "v1alpha5",
			vmopObj: &vmopv1alpha5.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       "VirtualMachine",
					APIVersion: vmopv1alpha5.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantHubObj: &vmopvhub.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       "VirtualMachine",
					APIVersion: vmopvhub.GroupVersion.String(),
				},
				Convertible: utilmeta.TypeMetaConvertible{
					APIVersion: vmopv1alpha5.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := versionAwareClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.vmopObj).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			hubObj := &vmopvhub.VirtualMachine{}
			err := c.Get(ctx, client.ObjectKeyFromObject(tt.vmopObj), hubObj)
			if (err != nil) != tt.wantErr {
				g.Fail(fmt.Sprintf("Get() error = %v, wantErr %v", err, tt.wantErr))
			}

			tt.wantHubObj.SetResourceVersion(hubObj.GetResourceVersion())
			g.Expect(hubObj).To(Equal(tt.wantHubObj))
		})
	}
}

func Test_versionAwareClient_List(t *testing.T) {
	tests := []struct {
		name             string
		preferredVersion string
		vmopObjs         []client.Object
		wantHubObjs      []client.Object
		wantErr          bool
	}{
		{
			name:             "Get VirtualMachine when preferred version is v1alpha2",
			preferredVersion: "v1alpha2",
			vmopObjs: []client.Object{
				&vmopv1alpha2.VirtualMachine{
					TypeMeta: metav1.TypeMeta{
						Kind:       "VirtualMachine",
						APIVersion: vmopv1alpha2.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: "test-ns",
					},
				},
			},
			wantHubObjs: []client.Object{
				&vmopvhub.VirtualMachine{
					TypeMeta: metav1.TypeMeta{
						Kind:       "VirtualMachine",
						APIVersion: vmopvhub.GroupVersion.String(),
					},
					Convertible: utilmeta.TypeMetaConvertible{
						APIVersion: vmopv1alpha2.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: "test-ns",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := versionAwareClient{
				internalClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.vmopObjs...).Build(),
				overrideGetPreferredVersion: func() string {
					return tt.preferredVersion
				},
			}

			hubObjs := &vmopvhub.VirtualMachineList{}
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
