// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.12.4
// source: pkg/apis/network/v1beta1/gateway.proto

package v1beta1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StreamKind int32

const (
	StreamKind_STREAM_STDIN  StreamKind = 0
	StreamKind_STREAM_STDOUT StreamKind = 1
	StreamKind_STREAM_STDERR StreamKind = 2
)

// Enum value maps for StreamKind.
var (
	StreamKind_name = map[int32]string{
		0: "STREAM_STDIN",
		1: "STREAM_STDOUT",
		2: "STREAM_STDERR",
	}
	StreamKind_value = map[string]int32{
		"STREAM_STDIN":  0,
		"STREAM_STDOUT": 1,
		"STREAM_STDERR": 2,
	}
)

func (x StreamKind) Enum() *StreamKind {
	p := new(StreamKind)
	*p = x
	return p
}

func (x StreamKind) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (StreamKind) Descriptor() protoreflect.EnumDescriptor {
	return file_pkg_apis_network_v1beta1_gateway_proto_enumTypes[0].Descriptor()
}

func (StreamKind) Type() protoreflect.EnumType {
	return &file_pkg_apis_network_v1beta1_gateway_proto_enumTypes[0]
}

func (x StreamKind) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use StreamKind.Descriptor instead.
func (StreamKind) EnumDescriptor() ([]byte, []int) {
	return file_pkg_apis_network_v1beta1_gateway_proto_rawDescGZIP(), []int{0}
}

type ExecRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Stdin []byte `protobuf:"bytes,1,opt,name=stdin,proto3" json:"stdin,omitempty"`
}

func (x *ExecRequest) Reset() {
	*x = ExecRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExecRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecRequest) ProtoMessage() {}

func (x *ExecRequest) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecRequest.ProtoReflect.Descriptor instead.
func (*ExecRequest) Descriptor() ([]byte, []int) {
	return file_pkg_apis_network_v1beta1_gateway_proto_rawDescGZIP(), []int{0}
}

func (x *ExecRequest) GetStdin() []byte {
	if x != nil {
		return x.Stdin
	}
	return nil
}

type ExecResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Stream []byte     `protobuf:"bytes,1,opt,name=stream,proto3" json:"stream,omitempty"`
	Kind   StreamKind `protobuf:"varint,2,opt,name=kind,proto3,enum=v1beta1.StreamKind" json:"kind,omitempty"`
}

func (x *ExecResponse) Reset() {
	*x = ExecResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExecResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecResponse) ProtoMessage() {}

func (x *ExecResponse) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecResponse.ProtoReflect.Descriptor instead.
func (*ExecResponse) Descriptor() ([]byte, []int) {
	return file_pkg_apis_network_v1beta1_gateway_proto_rawDescGZIP(), []int{1}
}

func (x *ExecResponse) GetStream() []byte {
	if x != nil {
		return x.Stream
	}
	return nil
}

func (x *ExecResponse) GetKind() StreamKind {
	if x != nil {
		return x.Kind
	}
	return StreamKind_STREAM_STDIN
}

var File_pkg_apis_network_v1beta1_gateway_proto protoreflect.FileDescriptor

var file_pkg_apis_network_v1beta1_gateway_proto_rawDesc = []byte{
	0x0a, 0x26, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x6e, 0x65, 0x74, 0x77, 0x6f,
	0x72, 0x6b, 0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x77,
	0x61, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61,
	0x31, 0x22, 0x23, 0x0a, 0x0b, 0x45, 0x78, 0x65, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x64, 0x69, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x05, 0x73, 0x74, 0x64, 0x69, 0x6e, 0x22, 0x4f, 0x0a, 0x0c, 0x45, 0x78, 0x65, 0x63, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x27,
	0x0a, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x76,
	0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x4b, 0x69, 0x6e,
	0x64, 0x52, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x2a, 0x44, 0x0a, 0x0a, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x4b, 0x69, 0x6e, 0x64, 0x12, 0x10, 0x0a, 0x0c, 0x53, 0x54, 0x52, 0x45, 0x41, 0x4d, 0x5f,
	0x53, 0x54, 0x44, 0x49, 0x4e, 0x10, 0x00, 0x12, 0x11, 0x0a, 0x0d, 0x53, 0x54, 0x52, 0x45, 0x41,
	0x4d, 0x5f, 0x53, 0x54, 0x44, 0x4f, 0x55, 0x54, 0x10, 0x01, 0x12, 0x11, 0x0a, 0x0d, 0x53, 0x54,
	0x52, 0x45, 0x41, 0x4d, 0x5f, 0x53, 0x54, 0x44, 0x45, 0x52, 0x52, 0x10, 0x02, 0x32, 0x44, 0x0a,
	0x07, 0x47, 0x61, 0x74, 0x65, 0x77, 0x61, 0x79, 0x12, 0x39, 0x0a, 0x04, 0x45, 0x78, 0x65, 0x63,
	0x12, 0x14, 0x2e, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2e, 0x45, 0x78, 0x65, 0x63, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x15, 0x2e, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31,
	0x2e, 0x45, 0x78, 0x65, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x28,
	0x01, 0x30, 0x01, 0x42, 0x33, 0x5a, 0x31, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f,
	0x6d, 0x2f, 0x72, 0x61, 0x66, 0x66, 0x69, 0x73, 0x2f, 0x72, 0x61, 0x67, 0x65, 0x74, 0x61, 0x2f,
	0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b,
	0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_pkg_apis_network_v1beta1_gateway_proto_rawDescOnce sync.Once
	file_pkg_apis_network_v1beta1_gateway_proto_rawDescData = file_pkg_apis_network_v1beta1_gateway_proto_rawDesc
)

func file_pkg_apis_network_v1beta1_gateway_proto_rawDescGZIP() []byte {
	file_pkg_apis_network_v1beta1_gateway_proto_rawDescOnce.Do(func() {
		file_pkg_apis_network_v1beta1_gateway_proto_rawDescData = protoimpl.X.CompressGZIP(file_pkg_apis_network_v1beta1_gateway_proto_rawDescData)
	})
	return file_pkg_apis_network_v1beta1_gateway_proto_rawDescData
}

var file_pkg_apis_network_v1beta1_gateway_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_pkg_apis_network_v1beta1_gateway_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_pkg_apis_network_v1beta1_gateway_proto_goTypes = []interface{}{
	(StreamKind)(0),      // 0: v1beta1.StreamKind
	(*ExecRequest)(nil),  // 1: v1beta1.ExecRequest
	(*ExecResponse)(nil), // 2: v1beta1.ExecResponse
}
var file_pkg_apis_network_v1beta1_gateway_proto_depIdxs = []int32{
	0, // 0: v1beta1.ExecResponse.kind:type_name -> v1beta1.StreamKind
	1, // 1: v1beta1.Gateway.Exec:input_type -> v1beta1.ExecRequest
	2, // 2: v1beta1.Gateway.Exec:output_type -> v1beta1.ExecResponse
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_pkg_apis_network_v1beta1_gateway_proto_init() }
func file_pkg_apis_network_v1beta1_gateway_proto_init() {
	if File_pkg_apis_network_v1beta1_gateway_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExecRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_pkg_apis_network_v1beta1_gateway_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExecResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_pkg_apis_network_v1beta1_gateway_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_pkg_apis_network_v1beta1_gateway_proto_goTypes,
		DependencyIndexes: file_pkg_apis_network_v1beta1_gateway_proto_depIdxs,
		EnumInfos:         file_pkg_apis_network_v1beta1_gateway_proto_enumTypes,
		MessageInfos:      file_pkg_apis_network_v1beta1_gateway_proto_msgTypes,
	}.Build()
	File_pkg_apis_network_v1beta1_gateway_proto = out.File
	file_pkg_apis_network_v1beta1_gateway_proto_rawDesc = nil
	file_pkg_apis_network_v1beta1_gateway_proto_goTypes = nil
	file_pkg_apis_network_v1beta1_gateway_proto_depIdxs = nil
}
