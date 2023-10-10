/*
Copyright 2023 Monocle developers.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MonocleSpec defines the desired state of Monocle
type MonocleSpec struct {
	// Fully Qualified Domain Name to access the Monocle Web UI
	FQDN string `json:"FQDN"`
	// Storage class name when creating the PVC
	StorageClassName string `json:"storageClassName,omitempty"`
	// Initial Storage Size for the database storage
	StorageSize string `json:"storageSize,omitempty"`
	// Monocle container image
	// +kubebuilder:default:="quay.io/change-metrics/monocle:1.8.0"
	MonocleImage string `json:"monocleImage,omitempty"`
}

// MonocleStatus defines the observed state of Monocle
type MonocleStatus struct {
	Elastic string `json:"monocle-elastic,omitempty"`
	Api     string `json:"monocle-api,omitempty"`
	Crawler string `json:"monocle-crawler,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Monocle is the Schema for the monocles API
type Monocle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonocleSpec   `json:"spec,omitempty"`
	Status MonocleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MonocleList contains a list of Monocle
type MonocleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Monocle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Monocle{}, &MonocleList{})
}
