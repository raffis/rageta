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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type RunnerClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunnerClaimSpec   `json:"spec,omitempty"`
	Status RunnerClaimStatus `json:"status,omitempty"`
}

type RunnerClaimSpec struct {
	RunnerPoolSelector metav1.LabelSelector        `json:"runnerPoolSelector,omitempty"`
	RunnerName         string                      `json:"runnerName,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
	AffinityKey        string                      `json:"affinityKey,omitempty"`
	Timeout            metav1.Duration             `json:"timeout,omitempty"`
}

type RunnerClaimStatus struct {
}

// +kubebuilder:object:root=true
type RunnerClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []RunnerClaim `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &RunnerClaim{}, &RunnerClaimList{})
}
