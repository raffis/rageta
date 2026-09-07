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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type RunnerVolumeClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

type RunnerVolumeClaimSpec struct {
	PVCTemplate  PVCTemplate `json:"pvcTemplate"`
	AffinityKeys []string    `json:"affinityKeys"`
}

type PVCTemplate struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMetadata `json:"metadata,omitempty"`

	// spec defines the desired characteristics of a volume requested by a runner.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	Spec v1.PersistentVolumeClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

type RunnerVolumeClaimStatus struct {
}

// +kubebuilder:object:root=true
type RunnerVolumeClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []RunnerVolumeClaim `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &RunnerVolumeClaim{}, &RunnerVolumeClaimList{})
}
