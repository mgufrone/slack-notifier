/*
Copyright 2025.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Status is the available status the controller manager would observer the current pod status
type Status string

const (
	StatusCrashLoopBackoff     Status = "CrashLoopBackoff"
	StatusImagePullBackOff     Status = "ImagePullBackOff"
	StatusOOMKilled            Status = "OOMKilled"
	StatusFailedMount          Status = "FailedMount"
	StatusContainerConfigError Status = "ContainerConfigError"
	StatusRunContainerError    Status = "RunContainerError"
	StatusPending              Status = "Pending"
	StatusUnknown              Status = "Unknown"
)

// SlackChannelSpec defines the desired state of SlackChannel.
type SlackChannelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Channel will be for the notification to send to
	Channel string `json:"channel"`
	// Status contains list of pod status to watch. default: OOMKilled, ImagePullBackoff, CrashLoopBackOff
	Status []Status `json:"status,omitempty"`
	// WatchAllNamespace would limit SlackChannel to only watch all pods within the same namespace. default Failed, Pending, Unknown
	WatchAllNamespace bool `json:"isNamespaceScoped,omitempty"`
}

// SlackChannelStatus defines the observed state of SlackChannel.
type SlackChannelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SlackChannel is the Schema for the slackchannels API.
type SlackChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlackChannelSpec   `json:"spec,omitempty"`
	Status SlackChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SlackChannelList contains a list of SlackChannel.
type SlackChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlackChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SlackChannel{}, &SlackChannelList{})
}
