/*

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// InstanceSpec defines the desired state of Instance
type InstanceSpec struct {
	Image         string `json:"image"`
	MaxRunSeconds *int64 `json:"maxRunSeconds"`

	// +optional
	Running *bool `json:"running,omitempty"`

	// +optional
	StorageClass *string `json:"storageClass,omitempty"`

	// +optional
	StorageCapacity *resource.Quantity `json:"storageCapacity,omitempty"`

	// +optional
	//GitCredentials InstanceGitCredentials `json:"gitCredentials,omitempty"`
}

type InstanceGitCredentials struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	GitUrl string `json:"gitUrl"`

	// +optional
	TokenSecret string `json:"tokenSecret,omitempty"`
	// +optional
	SshSecret string `json:"sshSecret,omitempty"`
}

type InstancePodState string

const (
	Starting InstancePodState = "Starting"
	Running  InstancePodState = "Running"
	Stopped  InstancePodState = "Stopped"
	Error    InstancePodState = "Error"
)

// InstanceStatus defines the observed state of Instance
type InstanceStatus struct {
	// +optional
	PodState InstancePodState `json:"podState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.podState",description="status of an instance"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Instance is the Schema for the instances API
type Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstanceSpec   `json:"spec,omitempty"`
	Status InstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InstanceList contains a list of Instance
type InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instance{}, &InstanceList{})
}
