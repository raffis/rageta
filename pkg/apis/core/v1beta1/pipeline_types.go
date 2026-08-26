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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type Pipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	PipelineSpec `json:",inline"`
}

type PipelineSpec struct {
	DefaultTask      string      `json:"defaultTask,omitempty"`
	ShortDescription string      `json:"shortDescription,omitempty"`
	LongDescription  string      `json:"longDescription,omitempty"`
	Inputs           InputParams `json:"inputs,omitempty"`
	Tasks            []Task      `json:"tasks,omitempty"`
	Templates        []Task      `json:"templates,omitempty"`
}

func (p Pipeline) SetDefaults() {
	for k := range p.Inputs {
		p.Inputs[k].SetDefaults()
	}
}

type TaskOptions struct {
	Templates    []LocalReference `json:"templates,omitempty"`
	When         []Condition      `json:"when,omitempty"`
	Hide         bool             `json:"expose,omitempty"`
	Exports      []Export         `json:"exports,omitempty"`
	Inputs       []InputParam     `json:"inputs,omitempty"`
	Timeout      metav1.Duration  `json:"timeout,omitempty"`
	AllowFailure bool             `json:"allowFailure,omitempty"`
	Matrix       *Matrix          `json:"matrix,omitempty"`
	DependsOn    []TaskDependency `json:"dependsOn,omitempty"`
	Retry        *Retry           `json:"retry,omitempty"`
	Secrets      []SecretVar      `json:"secrets,omitempty"`
	Env          []EnvVar         `json:"env,omitempty"`
	InputFrom    []InputFrom      `json:"inputFrom,omitempty"`
	EnvFrom      []EnvFrom        `json:"envFrom,omitempty"`
	Labels       []Label          `json:"labels,omitempty"`
	Sources      []Source         `json:"sources,omitempty"`
	VolumeMounts []VolumeMount    `json:"volumeMounts,omitempty"`
	Image        string           `json:"image,omitempty"`
	WorkingDir   string           `json:"workingDir,omitempty"`
}

type Export struct {
	Path *string `json:"path,omitempty"`
}

// +kubebuilder:validation:MinProperties=1
type TaskDependency struct {
	Name        string `json:"name,omitempty"`
	AwaitMatrix bool   `json:"awaitMatrix,omitempty"`
}

type Source struct {
	From *string `json:"from,omitempty"`
	Path string  `json:"path,omitempty"`
}

type Label struct {
	Name     string `json:"name,omitempty"`
	Value    string `json:"value,omitempty"`
	HEXColor string `json:"hexColor,omitempty"`
}

type SecretVar struct {
	Name  string  `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

type EnvVar struct {
	Name  string  `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

type InputFrom struct {
	File *string `json:"file,omitempty"`
}

type EnvFrom struct {
	File *string `json:"file,omitempty"`
}

type Condition struct {
	CelExpression *string `json:"celExpression,omitempty"`
}

type Matrix struct {
	Params        []Param        `json:"params,omitempty"`
	Include       []IncludeParam `json:"include,omitempty"`
	FailFast      bool           `json:"failFast,omitempty"`
	MaxConcurrent int            `json:"maxConcurrent,omitempty"`
}

type IncludeParam struct {
	Name   string      `json:"name,omitempty"`
	Params []Param     `json:"params,omitempty"`
	Label  MatrixLabel `json:"label,omitempty"`
}

type MatrixLabel struct {
	Value    string `json:"value,omitempty"`
	HEXColor string `json:"hexColor,omitempty"`
}

type Retry struct {
	Exponential metav1.Duration `json:"exponential,omitempty"`
	Constant    metav1.Duration `json:"constant,omitempty"`
	MaxRetries  int             `json:"maxRetries,omitempty"`
}

type Task struct {
	Name        string `json:"name,omitempty"`
	Short       string `json:"short,omitempty"`
	Long        string `json:"long,omitempty"`
	TaskOptions `json:",inline"`
	Steps       *[]Step      `json:"steps,omitempty"`
	Service     *ServiceTask `json:"service,omitempty"`
	Inherit     *InheritTask `json:"inherit,omitempty"`
}

type Step struct {
	Script string `json:"script,omitempty"`
}

type LocalReference struct {
	Name string `json:"name,omitempty"`
}

type VolumeMount struct {
	MountPath string          `json:"mountPath,omitempty"`
	ReadOnly  bool            `json:"readOnly,omitempty"`
	HostPath  *HostPathVolume `json:"hostPath,omitempty"`
	Cache     *CacheVolume    `json:"cache,omitempty"`
	TmpFS     *TmpFSVolume    `json:"tmpfs,omitempty"`
}

type HostPathVolume struct {
	// Path is the source path relative to the build context. May contain substitution expressions.
	Path string `json:"path,omitempty"`
}

type TmpFSVolume struct {
}

type CacheVolume struct {
	// Name is the cache namespace. May contain substitution expressions.
	Name string `json:"name,omitempty"`
	// Sharing controls concurrent access
	// +optional
	// +kubebuilder:validation:Enum=shared;private;locked
	Sharing string `json:"sharing,omitempty"`
}

type ServiceTask struct {
	Command []string            `json:"command,omitempty"`
	Args    []string            `json:"args,omitempty"`
	Uid     *intstr.IntOrString `json:"uid,omitempty"`
	Guid    *intstr.IntOrString `json:"guid,omitempty"`
}

type InheritTask struct {
	Pipeline string  `json:"pipeline,omitempty"`
	Task     string  `json:"task,omitempty"`
	Inputs   []Param `json:"inputs,omitempty"`
}

// +kubebuilder:object:root=true
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
