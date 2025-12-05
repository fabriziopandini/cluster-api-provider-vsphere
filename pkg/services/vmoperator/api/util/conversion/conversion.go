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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Convertible defines capability of a type to convertible i.e. it can be converted to/from a hub type.
type Convertible interface {
	client.Object
	ConvertTo(dst Hub) error
	ConvertFrom(src Hub) error
	Set(client.Object)
	GVK() schema.GroupVersionKind
}

// Hub marks that a given type is the hub type for conversion. This means that
// all conversions will first convert to the hub type, then convert from the hub
// type to the destination type. All types besides the hub type should implement
// Convertible.
type Hub interface {
	client.Object
	Hub()
	APIVersion() string
}
