// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation and Dapr Contributors.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// Subscription describes an pub/sub event subscription.
type Subscription struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SubscriptionSpec `json:"spec,omitempty"`
	// +optional
	Scopes []string `json:"scopes,omitempty"`
}

// SubscriptionSpec is the spec for an event subscription.
type SubscriptionSpec struct {
	// The PubSub component name.
	Pubsubname string `json:"pubsubname"`
	// The topic name to subscribe to.
	Topic string `json:"topic"`
	// The optional metadata to provide the the subscription.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
	// The list of routing rules for this topic.
	Routes []Route `json:"routes"`
}

// Route is used to specify the condition for sending
// a message to a specific path.
type Route struct {
	// The optional CEL expression used to match the event.
	// If the match is not specified, then the route is considered
	// the default. The rules are tested in the order specified,
	// so they should be define from most-to-least specific.
	// The default route should appear last in the list.
	// +optional
	Match string `json:"match"`

	// The path for events that match this rule.
	Path string `json:"path"`
}

// +kubebuilder:object:root=true

// SubscriptionList is a list of Dapr event sources.
type SubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Subscription `json:"items"`
}
