package v1beta1

type StepResult struct {
	Outputs map[string]ParamValue `protobuf:"bytes,1,rep,name=outputs,proto3" json:"outputs,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	TmpDir  string                `protobuf:"bytes,2,opt,name=tmpDir,proto3" json:"tmpDir,omitempty"`
}

type ContainerStatus struct {
	ContainerID string `protobuf:"bytes,1,opt,name=containerID,proto3" json:"containerID,omitempty"`
	ContainerIP string `protobuf:"bytes,2,opt,name=containerIP,proto3" json:"containerIP,omitempty"`
	Name        string `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
	Ready       bool   `protobuf:"varint,4,opt,name=ready,proto3" json:"ready,omitempty"`
	Started     bool   `protobuf:"varint,5,opt,name=started,proto3" json:"started,omitempty"`
	ExitCode    int32  `protobuf:"varint,6,opt,name=exitCode,proto3" json:"exitCode,omitempty"`
}

type Output struct {
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
}

type RuntimeVars struct {
	Args       []string                    `protobuf:"bytes,1,rep,name=args,proto3" json:"args,omitempty"`
	Inputs     map[string]ParamValue       `protobuf:"bytes,2,rep,name=inputs,proto3" json:"inputs,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Envs       map[string]string           `protobuf:"bytes,3,rep,name=envs,proto3" json:"envs,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Containers map[string]*ContainerStatus `protobuf:"bytes,4,rep,name=containers,proto3" json:"containers,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Steps      map[string]*StepResult      `protobuf:"bytes,5,rep,name=steps,proto3" json:"steps,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	TmpDir     string                      `protobuf:"bytes,6,opt,name=tmpDir,proto3" json:"tmpDir,omitempty"`
	Matrix     map[string]string           `protobuf:"bytes,7,rep,name=matrix,proto3" json:"matrix,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Env        string                      `protobuf:"bytes,8,opt,name=env,proto3" json:"env,omitempty"`
	Outputs    map[string]*Output          `protobuf:"bytes,9,rep,name=outputs,proto3" json:"outputs,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Os         string                      `protobuf:"bytes,10,opt,name=os,proto3" json:"os,omitempty"`
	Arch       string                      `protobuf:"bytes,11,opt,name=arch,proto3" json:"arch,omitempty"`
}
