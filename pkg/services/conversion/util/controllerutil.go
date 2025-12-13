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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	conversionclient "sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/conversion/client"
)

// FIXME: consider if to move CreateOrPatch int client pkg.

// CreateOrPatch attempts to fetch the given object from the Kubernetes cluster.
// If the object didn't exist, MutateFn will be called, and it will be created.
// If the object did exist, MutateFn will be called, and if it changed the
// object, it will be patched.
// Otherwise, it will be left unchanged.
// The executed operation (and an error) will be returned.
//
// WARNING: If the MutateFn resets a value on obj that has a default value,
// CreateOrPatch will *always* perform a patch. This is because when the
// object is fetched from the API server, the value will have taken on the
// default value, and the check for equality will fail.
// For example, Deployments must have a Replicas value set. If the MutateFn sets
// a Deployment's Replicas to nil, then it will never match with the object
// returned from the API server, which defaults the value to 1.
//
// WARNING: CreateOrPatch assumes that no values have been set on obj aside
// from the Name/Namespace. Values other than Name and Namespace that existed on
// obj may be overwritten by the corresponding values in the object returned
// from the Kubernetes API server. When this happens, the Patch will not work
// as expected.
//
// Note: changes to any sub-resource other than status will be ignored.
// Changes to the status sub-resource will only be applied if the object
// already exist. To change the status on object creation, the easiest
// way is to requeue the object in the controller if OperationResult is
// OperationResultCreated.
func CreateOrPatch(ctx context.Context, c client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := c.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		if f != nil {
			if err := mutate(f, key, obj); err != nil {
				return controllerutil.OperationResultNone, err
			}
		}
		if err := c.Create(ctx, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultCreated, nil
	}

	// Create patches for the object and its possible status.
	objPatch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	statusPatch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	if patchCreator, ok := obj.(conversionclient.MergePatchCreator); ok {
		var err error
		objPatch, err = patchCreator.MergeFrom(obj.DeepCopyObject().(client.Object))
		if err != nil {
			return controllerutil.OperationResultNone, err
		}
		statusPatch, err = patchCreator.MergeFrom(obj.DeepCopyObject().(client.Object))
		if err != nil {
			return controllerutil.OperationResultNone, err
		}
	}

	// Create a copy of the original object as well as converting that copy to
	// unstructured data.
	before, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj.DeepCopyObject())
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// Attempt to extract the status from the resource for easier comparison later
	beforeStatus, hasBeforeStatus, err := unstructured.NestedFieldCopy(before, "status")
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// If the resource contains a status then remove it from the unstructured
	// copy to avoid unnecessary patching later.
	if hasBeforeStatus {
		unstructured.RemoveNestedField(before, "status")
	}

	// Mutate the original object.
	if f != nil {
		if err := mutate(f, key, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
	}

	// Convert the resource to unstructured to compare against our before copy.
	after, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// Attempt to extract the status from the resource for easier comparison later
	afterStatus, hasAfterStatus, err := unstructured.NestedFieldCopy(after, "status")
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// If the resource contains a status then remove it from the unstructured
	// copy to avoid unnecessary patching later.
	if hasAfterStatus {
		unstructured.RemoveNestedField(after, "status")
	}

	result := controllerutil.OperationResultNone

	if !reflect.DeepEqual(before, after) {
		// Only issue a Patch if the before and after resources (minus status) differ
		if err := c.Patch(ctx, obj, objPatch); err != nil {
			return result, err
		}
		result = controllerutil.OperationResultUpdated
	}

	if (hasBeforeStatus || hasAfterStatus) && !reflect.DeepEqual(beforeStatus, afterStatus) {
		// Only issue a Status Patch if the resource has a status and the beforeStatus
		// and afterStatus copies differ
		if result == controllerutil.OperationResultUpdated {
			// If Status was replaced by Patch before, set it to afterStatus
			objectAfterPatch, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return result, err
			}
			if err = unstructured.SetNestedField(objectAfterPatch, afterStatus, "status"); err != nil {
				return result, err
			}
			// If Status was replaced by Patch before, restore patched structure to the obj
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(objectAfterPatch, obj); err != nil {
				return result, err
			}
		}
		if err := c.Status().Patch(ctx, obj, statusPatch); err != nil {
			return result, err
		}
		if result == controllerutil.OperationResultUpdated {
			result = controllerutil.OperationResultUpdatedStatus
		} else {
			result = controllerutil.OperationResultUpdatedStatusOnly
		}
	}

	return result, nil
}

// mutate wraps a MutateFn and applies validation to its result.
func mutate(f controllerutil.MutateFn, key client.ObjectKey, obj client.Object) error {
	if err := f(); err != nil {
		return err
	}
	if newKey := client.ObjectKeyFromObject(obj); key != newKey {
		return fmt.Errorf("MutateFn cannot mutate object name and/or object namespace")
	}
	return nil
}
