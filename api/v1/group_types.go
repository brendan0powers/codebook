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

// GroupSpec defines the desired state of Group
type GroupSpec struct {
	Image         string   `json:"image"`
	Instances     []string `json:"instances"`
	MaxRunSeconds *int64   `json:"maxRunSeconds"`

	// +optional
	StorageClass *string `json:"storageClass,omitempty"`

	// +optional
	StorageCapacity *resource.Quantity `json:"storageCapacity,omitempty"`
}

type GroupInstanceStatus struct {
	Name   string           `json:"name"`
	Status InstancePodState `json:"status"`
}

// GroupStatus defines the observed state of Group
type GroupStatus struct {
	// +optional
	Instances []GroupInstanceStatus `json:"instances,omitempty"`

	NumInstances        *int64 `json:"numInstances"`
	NumRunningInstances *int64 `json:"numRunningInstances"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Instances",type="integer",JSONPath=".status.numInstances",description="Number of instances in group"
// +kubebuilder:printcolumn:name="Running",type="integer",JSONPath=".status.numRunningInstances",description="Number of running instances in group"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Group is the Schema for the groups API
type Group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GroupSpec   `json:"spec,omitempty"`
	Status GroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupList contains a list of Group
type GroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Group `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Group{}, &GroupList{})
}
