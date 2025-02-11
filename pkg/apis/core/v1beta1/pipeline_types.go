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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
type Pipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	PipelineSpec `json:",inline"`
}

type PipelineSpec struct {
	Inherits         []string       `json:"inherits,omitempty"`
	Name             string         `json:"name,omitempty"`
	Entrypoint       string         `json:"entrypoint,omitempty"`
	ShortDescription string         `json:"shortDescription,omitempty"`
	LongDescription  string         `json:"longDescription,omitempty"`
	Inputs           []Input        `json:"inputs,omitempty"`
	Steps            []Step         `json:"steps,omitempty"`
	SubPipelines     []PipelineSpec `json:"subPipelines,omitempty"`
}

type Input struct {
	Name        string          `json:"name,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Description string          `json:"description,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
	Type        InputType       `json:"type,omitempty"`
}

type InputType string

var (
	InputTypeStringSlice InputType = "[]string"
	InputTypeString      InputType = "string"
	InputTypeBool        InputType = "bool"
)

type StepOptions struct {
	If           string          `json:"if,omitempty"`
	Timeout      metav1.Duration `json:"timeout,omitempty"`
	Finally      bool            `json:"finally,omitempty"`
	AllowFailure bool            `json:"allowFailure,omitempty"`
	Matrix       *Matrix         `json:"matrix,omitempty"`
	Generates    []Generate      `json:"generates,omitempty"`
	Sources      []Source        `json:"sources,omitempty"`
	Needs        []StepReference `json:"needs,omitempty"`
	Streams      *Streams        `json:"streams,omitempty"`
	Retry        *Retry          `json:"retry,omitempty"`
	Env          []string        `json:"env,omitempty"`
}

type Matrix struct {
	Table    map[string]json.RawMessage `json:"table,omitempty"`
	FailFast bool                       `json:"failFast,omitempty"`
}

type Retry struct {
	Exponential metav1.Duration `json:"exponential,omitempty"`
	Constant    metav1.Duration `json:"constant,omitempty"`
	MaxRetries  int             `json:"maxRetries,omitempty"`
}

type Source struct {
	Match string `json:"match,omitempty"`
}

type Generate struct {
	Path string `json:"path,omitempty"`
}

type Step struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	StepOptions `json:",inline"`
	Pipe        *PipeStep       `json:"pipe,omitempty"`
	And         *AndStep        `json:"and,omitempty"`
	Concurrent  *ConcurrentStep `json:"concurrent,omitempty"`
	Run         *RunStep        `json:"run,omitempty"`
	Inherit     *InheritStep    `json:"inherit,omitempty"`
}
type AndStep struct {
	Refs []StepReference `json:"refs,omitempty"`
}

type StepReference struct {
	Name string `json:"name,omitempty"`
}

type ConcurrentStep struct {
	FailFast bool            `json:"failFast,omitempty"`
	Refs     []StepReference `json:"refs,omitempty"`
}

type PipeStep struct {
	Refs []StepReference `json:"refs,omitempty"`
}

type RunStep struct {
	Await     AwaitStatus `json:"await,omitempty"`
	Streams   Streams     `json:"streams,omitempty"`
	Container `json:",inline"`
}

type Container struct {
	Stdin         bool          `json:"stdin,omitempty"`
	TTY           bool          `json:"tty,omitempty"`
	Image         string        `json:"image,omitempty"`
	Command       []string      `json:"command,omitempty"`
	Args          []string      `json:"args,omitempty"`
	PWD           string        `json:"pwd,omitempty"`
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`
}

type AwaitStatus string

var (
	AwaitStatusReady AwaitStatus = "Ready"
	AwaitStatusExit  AwaitStatus = "Exit"
)

type RestartPolicy string

var (
	RestartPolicyNever     RestartPolicy = "Never"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
	RestartPolicyAlways    RestartPolicy = "Always"
)

type InheritStep struct {
	Pipeline   string         `json:"pipeline,omitempty"`
	Entrypoint string         `json:"entrypoint,omitempty"`
	Inputs     []InheritInput `json:"inputs,omitempty"`
}

type InheritInput struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Streams struct {
	Stdout *Stream `json:"stdout,omitempty"`
	Stdin  *Stream `json:"stdin,omitempty"`
	Stderr *Stream `json:"stderr,omitempty"`
}

type Stream struct {
	Path   string `json:"path,omitempty"`
	Append bool   `json:"append,omitempty"`
}

// +kubebuilder:object:root=true
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
