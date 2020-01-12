// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	values_v1alpha1 "github.com/dapr/dapr/pkg/apis/values/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Component describes an Dapr component type
type Component struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec ComponentSpec `json:"spec,omitempty"`
	// +optional
	Auth `json:"auth,omitempty"`
}

// ComponentSpec is the spec for a component
type ComponentSpec struct {
	Type   string      `json:"type"`
	Config values_v1alpha1.Values `json:"config"`
}

// Auth represents authentication details for the component
type Auth struct {
	SecretStore string `json:"secretStore"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ComponentList is a list of Dapr components
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Component `json:"items"`
}
