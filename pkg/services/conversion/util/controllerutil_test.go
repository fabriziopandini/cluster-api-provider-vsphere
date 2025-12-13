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

package util_test

import (
	"context"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	conversionutil "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/util"
)

var _ = Describe("Controllerutil", func() {
	Describe("CreateOrPatch", func() {
		var deploy *appsv1.Deployment
		var deplSpec appsv1.DeploymentSpec
		var deplKey types.NamespacedName
		var specr controllerutil.MutateFn

		BeforeEach(func() {
			deploy = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("deploy-%d", rand.Int31()), //nolint:gosec
					Namespace: "default",
				},
			}

			deplSpec = appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "busybox",
								Image: "busybox",
							},
						},
					},
				},
			}

			deplKey = types.NamespacedName{
				Name:      deploy.Name,
				Namespace: deploy.Namespace,
			}

			specr = deploymentSpecr(deploy, deplSpec)
		})

		assertLocalDeployWasUpdated := func(ctx context.Context, fetched *appsv1.Deployment) {
			By("local deploy object was updated during patch & has same spec, status, resource version as fetched")
			if fetched == nil {
				fetched = &appsv1.Deployment{}
				ExpectWithOffset(1, c.Get(ctx, deplKey, fetched)).To(Succeed())
			}
			ExpectWithOffset(1, fetched.ResourceVersion).To(Equal(deploy.ResourceVersion))
			ExpectWithOffset(1, fetched.Spec).To(BeEquivalentTo(deploy.Spec))
			ExpectWithOffset(1, fetched.Status).To(BeEquivalentTo(deploy.Status))
		}

		assertLocalDeployStatusWasUpdated := func(ctx context.Context, fetched *appsv1.Deployment) {
			By("local deploy object was updated during patch & has same spec, status, resource version as fetched")
			if fetched == nil {
				fetched = &appsv1.Deployment{}
				ExpectWithOffset(1, c.Get(ctx, deplKey, fetched)).To(Succeed())
			}
			ExpectWithOffset(1, fetched.ResourceVersion).To(Equal(deploy.ResourceVersion))
			ExpectWithOffset(1, *fetched.Spec.Replicas).To(BeEquivalentTo(int32(5)))
			ExpectWithOffset(1, fetched.Status).To(BeEquivalentTo(deploy.Status))
			ExpectWithOffset(1, len(fetched.Status.Conditions)).To(BeEquivalentTo(1))
		}

		It("creates a new object if one doesn't exists", func(ctx SpecContext) {
			op, err := conversionutil.CreateOrPatch(ctx, c, deploy, specr)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultCreated")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))

			By("actually having the deployment created")
			fetched := &appsv1.Deployment{}
			Expect(c.Get(ctx, deplKey, fetched)).To(Succeed())

			By("being mutated by MutateFn")
			Expect(fetched.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(fetched.Spec.Template.Spec.Containers[0].Name).To(Equal(deplSpec.Template.Spec.Containers[0].Name))
			Expect(fetched.Spec.Template.Spec.Containers[0].Image).To(Equal(deplSpec.Template.Spec.Containers[0].Image))
		})

		It("patches existing object", func(ctx SpecContext) {
			var scale int32 = 2
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)
			Expect(err).NotTo(HaveOccurred())
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))

			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, deploymentScaler(deploy, scale))
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdated")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdated))

			By("actually having the deployment scaled")
			fetched := &appsv1.Deployment{}
			Expect(c.Get(ctx, deplKey, fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(scale))
			assertLocalDeployWasUpdated(ctx, fetched)
		})

		It("patches only changed objects", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, deploymentIdentity)
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))

			assertLocalDeployWasUpdated(ctx, nil)
		})

		It("patches only changed status", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			deployStatus := appsv1.DeploymentStatus{
				ReadyReplicas: 1,
				Replicas:      3,
			}
			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, deploymentStatusr(deploy, deployStatus))
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdatedStatusOnly")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdatedStatusOnly))

			assertLocalDeployWasUpdated(ctx, nil)
		})

		It("patches resource and status", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			replicas := int32(3)
			deployStatus := appsv1.DeploymentStatus{
				ReadyReplicas: 1,
				Replicas:      replicas,
			}
			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, func() error {
				Expect(deploymentScaler(deploy, replicas)()).To(Succeed())
				return deploymentStatusr(deploy, deployStatus)()
			})
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdatedStatus")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdatedStatus))

			assertLocalDeployWasUpdated(ctx, nil)
		})

		It("patches resource and not empty status", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			replicas := int32(3)
			deployStatus := appsv1.DeploymentStatus{
				ReadyReplicas: 1,
				Replicas:      replicas,
			}
			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, func() error {
				Expect(deploymentScaler(deploy, replicas)()).To(Succeed())
				return deploymentStatusr(deploy, deployStatus)()
			})
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdatedStatus")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdatedStatus))

			assertLocalDeployWasUpdated(ctx, nil)

			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, func() error {
				deploy.Spec.Replicas = ptr.To(int32(5))
				deploy.Status.Conditions = []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
				}}
				return nil
			})
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdatedStatus")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdatedStatus))

			assertLocalDeployStatusWasUpdated(ctx, nil)
		})

		It("errors when MutateFn changes object name on creation", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, func() error {
				Expect(specr()).To(Succeed())
				return deploymentRenamer(deploy)()
			})

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("errors when MutateFn renames an object", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, deploymentRenamer(deploy))

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("errors when object namespace changes", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrPatch(ctx, c, deploy, deploymentNamespaceChanger(deploy))

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("aborts immediately if there was an error initially retrieving the object", func(ctx SpecContext) {
			op, err := controllerutil.CreateOrPatch(ctx, errorReader{c}, deploy, func() error {
				Fail("Mutation method should not run")
				return nil
			})

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
			Expect(err).To(HaveOccurred())
		})
	})
})

var (
	_ runtime.Object = &errRuntimeObj{}
	_ metav1.Object  = &errMetaObj{}
)

type errRuntimeObj struct {
	runtime.TypeMeta
}

func (o *errRuntimeObj) DeepCopyObject() runtime.Object {
	return &errRuntimeObj{}
}

type errMetaObj struct {
	metav1.ObjectMeta
}

func deploymentSpecr(deploy *appsv1.Deployment, spec appsv1.DeploymentSpec) controllerutil.MutateFn {
	return func() error {
		deploy.Spec = spec
		return nil
	}
}

func deploymentStatusr(deploy *appsv1.Deployment, status appsv1.DeploymentStatus) controllerutil.MutateFn {
	return func() error {
		deploy.Status = status
		return nil
	}
}

var deploymentIdentity controllerutil.MutateFn = func() error {
	return nil
}

func deploymentRenamer(deploy *appsv1.Deployment) controllerutil.MutateFn {
	return func() error {
		deploy.Name = fmt.Sprintf("%s-1", deploy.Name)
		return nil
	}
}

func deploymentNamespaceChanger(deploy *appsv1.Deployment) controllerutil.MutateFn {
	return func() error {
		deploy.Namespace = fmt.Sprintf("%s-1", deploy.Namespace)
		return nil
	}
}

func deploymentScaler(deploy *appsv1.Deployment, replicas int32) controllerutil.MutateFn {
	fn := func() error {
		deploy.Spec.Replicas = &replicas
		return nil
	}
	return fn
}

type errorReader struct {
	client.Client
}

func (e errorReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return fmt.Errorf("unexpected error")
}
