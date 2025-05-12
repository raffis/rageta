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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
type PipelineRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	PipelineRunSpec `json:",inline"`
}

type PipelineRunSpec struct {
	TTL                 metav1.Duration `json:"ttl,omitempty"`
	Timeout             metav1.Duration `json:"timeout,omitempty"`
	Pipeline            string          `json:"pipeline,omitempty"`
	Entrypoint          string          `json:"entrypoint,omitempty"`
	Inputs              []Param         `json:"inputs,omitempty"`
	GracefulTermination metav1.Duration `json:"gracefulTermination,omitempty"`
	PodTemplate         *PodTemplate    `json:"podTemplate,omitempty"`
	SkipDone            bool            `json:"skipDone,omitempty"`
	SkipSteps           []string        `json:"skipSteps,omitempty"`
	LogDetached         bool            `json:"logsDetached,omitempty"`
	MaxConcurrent       int             `json:"maxConcurrent,omitempty"`
	Decouple            bool            `json:"decouple,omitempty"`
	NoProgress          bool            `json:"noProgress,omitempty"`
	WithInternals       bool            `json:"withInternals,omitempty"`
	User                string          `json:"user,omitempty"`
}

type PodTemplate struct {
}

// +kubebuilder:object:root=true
type PipelineRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PipelineRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PipelineRun{}, &PipelineRunList{})
}
