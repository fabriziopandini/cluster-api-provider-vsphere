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

// FIXME: rules for import name consistency, import rule restrictions (no vmoprv1 outside of this folder)

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConvertibleWrapper defines a wrapper to an object that make the object convertible i.e. it can be converted to/from a hub type.
type ConvertibleWrapper interface {
	GroupVersionKind() schema.GroupVersionKind
	ConvertTo(src runtime.Object, dst runtime.Object) error
	ConvertFrom(src runtime.Object, dst runtime.Object) error
}

// Hub marks that a given type is the hub type for conversion.
type Hub interface {
	client.Object
	Hub()
	// FIXME: think about name
	SetConvertibleAPIVersion(string)
	GetConvertibleAPIVersion() string
}
