// Code generated by protoc-gen-go. DO NOT EDIT.
// source: proxy.proto

package api

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type InitializeBuildEnvRequest struct {
	Project              *Project `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	ContainerImage       string   `protobuf:"bytes,2,opt,name=container_image,json=containerImage,proto3" json:"container_image,omitempty"`
	WorkerNum            int32    `protobuf:"varint,3,opt,name=worker_num,json=workerNum,proto3" json:"worker_num,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *InitializeBuildEnvRequest) Reset()         { *m = InitializeBuildEnvRequest{} }
func (m *InitializeBuildEnvRequest) String() string { return proto.CompactTextString(m) }
func (*InitializeBuildEnvRequest) ProtoMessage()    {}
func (*InitializeBuildEnvRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{0}
}

func (m *InitializeBuildEnvRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_InitializeBuildEnvRequest.Unmarshal(m, b)
}
func (m *InitializeBuildEnvRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_InitializeBuildEnvRequest.Marshal(b, m, deterministic)
}
func (m *InitializeBuildEnvRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InitializeBuildEnvRequest.Merge(m, src)
}
func (m *InitializeBuildEnvRequest) XXX_Size() int {
	return xxx_messageInfo_InitializeBuildEnvRequest.Size(m)
}
func (m *InitializeBuildEnvRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_InitializeBuildEnvRequest.DiscardUnknown(m)
}

var xxx_messageInfo_InitializeBuildEnvRequest proto.InternalMessageInfo

func (m *InitializeBuildEnvRequest) GetProject() *Project {
	if m != nil {
		return m.Project
	}
	return nil
}

func (m *InitializeBuildEnvRequest) GetContainerImage() string {
	if m != nil {
		return m.ContainerImage
	}
	return ""
}

func (m *InitializeBuildEnvRequest) GetWorkerNum() int32 {
	if m != nil {
		return m.WorkerNum
	}
	return 0
}

type InitializeBuildEnvResponse struct {
	Status               *Status  `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	Peers                []*Peer  `protobuf:"bytes,2,rep,name=peers,proto3" json:"peers,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *InitializeBuildEnvResponse) Reset()         { *m = InitializeBuildEnvResponse{} }
func (m *InitializeBuildEnvResponse) String() string { return proto.CompactTextString(m) }
func (*InitializeBuildEnvResponse) ProtoMessage()    {}
func (*InitializeBuildEnvResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{1}
}

func (m *InitializeBuildEnvResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_InitializeBuildEnvResponse.Unmarshal(m, b)
}
func (m *InitializeBuildEnvResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_InitializeBuildEnvResponse.Marshal(b, m, deterministic)
}
func (m *InitializeBuildEnvResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InitializeBuildEnvResponse.Merge(m, src)
}
func (m *InitializeBuildEnvResponse) XXX_Size() int {
	return xxx_messageInfo_InitializeBuildEnvResponse.Size(m)
}
func (m *InitializeBuildEnvResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_InitializeBuildEnvResponse.DiscardUnknown(m)
}

var xxx_messageInfo_InitializeBuildEnvResponse proto.InternalMessageInfo

func (m *InitializeBuildEnvResponse) GetStatus() *Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *InitializeBuildEnvResponse) GetPeers() []*Peer {
	if m != nil {
		return m.Peers
	}
	return nil
}

type ForwardAndExecuteRequest struct {
	Project              *Project `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	CmdId                string   `protobuf:"bytes,2,opt,name=cmd_id,json=cmdId,proto3" json:"cmd_id,omitempty"`
	CmdContent           string   `protobuf:"bytes,3,opt,name=cmd_content,json=cmdContent,proto3" json:"cmd_content,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ForwardAndExecuteRequest) Reset()         { *m = ForwardAndExecuteRequest{} }
func (m *ForwardAndExecuteRequest) String() string { return proto.CompactTextString(m) }
func (*ForwardAndExecuteRequest) ProtoMessage()    {}
func (*ForwardAndExecuteRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{2}
}

func (m *ForwardAndExecuteRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ForwardAndExecuteRequest.Unmarshal(m, b)
}
func (m *ForwardAndExecuteRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ForwardAndExecuteRequest.Marshal(b, m, deterministic)
}
func (m *ForwardAndExecuteRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ForwardAndExecuteRequest.Merge(m, src)
}
func (m *ForwardAndExecuteRequest) XXX_Size() int {
	return xxx_messageInfo_ForwardAndExecuteRequest.Size(m)
}
func (m *ForwardAndExecuteRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ForwardAndExecuteRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ForwardAndExecuteRequest proto.InternalMessageInfo

func (m *ForwardAndExecuteRequest) GetProject() *Project {
	if m != nil {
		return m.Project
	}
	return nil
}

func (m *ForwardAndExecuteRequest) GetCmdId() string {
	if m != nil {
		return m.CmdId
	}
	return ""
}

func (m *ForwardAndExecuteRequest) GetCmdContent() string {
	if m != nil {
		return m.CmdContent
	}
	return ""
}

type ForwardAndExecuteResponse struct {
	Status               *Status  `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	Id                   string   `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	Executor             *Peer    `protobuf:"bytes,3,opt,name=executor,proto3" json:"executor,omitempty"`
	StdOut               string   `protobuf:"bytes,4,opt,name=std_out,json=stdOut,proto3" json:"std_out,omitempty"`
	StdErr               string   `protobuf:"bytes,5,opt,name=std_err,json=stdErr,proto3" json:"std_err,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ForwardAndExecuteResponse) Reset()         { *m = ForwardAndExecuteResponse{} }
func (m *ForwardAndExecuteResponse) String() string { return proto.CompactTextString(m) }
func (*ForwardAndExecuteResponse) ProtoMessage()    {}
func (*ForwardAndExecuteResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{3}
}

func (m *ForwardAndExecuteResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ForwardAndExecuteResponse.Unmarshal(m, b)
}
func (m *ForwardAndExecuteResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ForwardAndExecuteResponse.Marshal(b, m, deterministic)
}
func (m *ForwardAndExecuteResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ForwardAndExecuteResponse.Merge(m, src)
}
func (m *ForwardAndExecuteResponse) XXX_Size() int {
	return xxx_messageInfo_ForwardAndExecuteResponse.Size(m)
}
func (m *ForwardAndExecuteResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ForwardAndExecuteResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ForwardAndExecuteResponse proto.InternalMessageInfo

func (m *ForwardAndExecuteResponse) GetStatus() *Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *ForwardAndExecuteResponse) GetId() string {
	if m != nil {
		return m.Id
	}
	return ""
}

func (m *ForwardAndExecuteResponse) GetExecutor() *Peer {
	if m != nil {
		return m.Executor
	}
	return nil
}

func (m *ForwardAndExecuteResponse) GetStdOut() string {
	if m != nil {
		return m.StdOut
	}
	return ""
}

func (m *ForwardAndExecuteResponse) GetStdErr() string {
	if m != nil {
		return m.StdErr
	}
	return ""
}

type ClearBuildEnvRequest struct {
	Project              *Project `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ClearBuildEnvRequest) Reset()         { *m = ClearBuildEnvRequest{} }
func (m *ClearBuildEnvRequest) String() string { return proto.CompactTextString(m) }
func (*ClearBuildEnvRequest) ProtoMessage()    {}
func (*ClearBuildEnvRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{4}
}

func (m *ClearBuildEnvRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ClearBuildEnvRequest.Unmarshal(m, b)
}
func (m *ClearBuildEnvRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ClearBuildEnvRequest.Marshal(b, m, deterministic)
}
func (m *ClearBuildEnvRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ClearBuildEnvRequest.Merge(m, src)
}
func (m *ClearBuildEnvRequest) XXX_Size() int {
	return xxx_messageInfo_ClearBuildEnvRequest.Size(m)
}
func (m *ClearBuildEnvRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ClearBuildEnvRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ClearBuildEnvRequest proto.InternalMessageInfo

func (m *ClearBuildEnvRequest) GetProject() *Project {
	if m != nil {
		return m.Project
	}
	return nil
}

type ClearBuildEnvResponse struct {
	Status               *Status  `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ClearBuildEnvResponse) Reset()         { *m = ClearBuildEnvResponse{} }
func (m *ClearBuildEnvResponse) String() string { return proto.CompactTextString(m) }
func (*ClearBuildEnvResponse) ProtoMessage()    {}
func (*ClearBuildEnvResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_700b50b08ed8dbaf, []int{5}
}

func (m *ClearBuildEnvResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ClearBuildEnvResponse.Unmarshal(m, b)
}
func (m *ClearBuildEnvResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ClearBuildEnvResponse.Marshal(b, m, deterministic)
}
func (m *ClearBuildEnvResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ClearBuildEnvResponse.Merge(m, src)
}
func (m *ClearBuildEnvResponse) XXX_Size() int {
	return xxx_messageInfo_ClearBuildEnvResponse.Size(m)
}
func (m *ClearBuildEnvResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ClearBuildEnvResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ClearBuildEnvResponse proto.InternalMessageInfo

func (m *ClearBuildEnvResponse) GetStatus() *Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func init() {
	proto.RegisterType((*InitializeBuildEnvRequest)(nil), "api.InitializeBuildEnvRequest")
	proto.RegisterType((*InitializeBuildEnvResponse)(nil), "api.InitializeBuildEnvResponse")
	proto.RegisterType((*ForwardAndExecuteRequest)(nil), "api.ForwardAndExecuteRequest")
	proto.RegisterType((*ForwardAndExecuteResponse)(nil), "api.ForwardAndExecuteResponse")
	proto.RegisterType((*ClearBuildEnvRequest)(nil), "api.ClearBuildEnvRequest")
	proto.RegisterType((*ClearBuildEnvResponse)(nil), "api.ClearBuildEnvResponse")
}

func init() {
	proto.RegisterFile("proxy.proto", fileDescriptor_700b50b08ed8dbaf)
}

var fileDescriptor_700b50b08ed8dbaf = []byte{
	// 434 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x93, 0x41, 0x6f, 0xd3, 0x40,
	0x10, 0x85, 0x65, 0x07, 0x27, 0x64, 0x5c, 0x5a, 0xb1, 0xa2, 0xc2, 0xb1, 0xd4, 0x26, 0x32, 0x02,
	0x72, 0x0a, 0x52, 0xb8, 0x22, 0x24, 0x5a, 0xa5, 0x52, 0x2e, 0x50, 0xb9, 0xe2, 0xc2, 0x25, 0xda,
	0x7a, 0x47, 0xb0, 0x10, 0xef, 0x9a, 0xd9, 0x5d, 0x5a, 0x7a, 0xe6, 0xc6, 0x2f, 0xe1, 0x5f, 0xa2,
	0xec, 0xba, 0x91, 0x4a, 0x93, 0x43, 0xb8, 0x7e, 0x4f, 0x33, 0xf3, 0x9e, 0xfd, 0x16, 0xd2, 0x86,
	0xf4, 0xf5, 0xcf, 0x49, 0x43, 0xda, 0x6a, 0xd6, 0xe1, 0x8d, 0xcc, 0xf7, 0x2a, 0x5d, 0xd7, 0x5a,
	0x05, 0x54, 0xfc, 0x8e, 0x60, 0x30, 0x57, 0xd2, 0x4a, 0xbe, 0x94, 0x37, 0x78, 0xe2, 0xe4, 0x52,
	0xcc, 0xd4, 0x8f, 0x12, 0xbf, 0x3b, 0x34, 0x96, 0xbd, 0x80, 0x5e, 0x43, 0xfa, 0x2b, 0x56, 0x36,
	0x8b, 0x46, 0xd1, 0x38, 0x9d, 0xee, 0x4d, 0x78, 0x23, 0x27, 0xe7, 0x81, 0x95, 0xb7, 0x22, 0x7b,
	0x09, 0x07, 0x95, 0x56, 0x96, 0x4b, 0x85, 0xb4, 0x90, 0x35, 0xff, 0x8c, 0x59, 0x3c, 0x8a, 0xc6,
	0xfd, 0x72, 0x7f, 0x8d, 0xe7, 0x2b, 0xca, 0x8e, 0x00, 0xae, 0x34, 0x7d, 0x43, 0x5a, 0x28, 0x57,
	0x67, 0x9d, 0x51, 0x34, 0x4e, 0xca, 0x7e, 0x20, 0xef, 0x5d, 0x5d, 0x5c, 0x42, 0xbe, 0xc9, 0x8c,
	0x69, 0xb4, 0x32, 0xc8, 0x9e, 0x41, 0xd7, 0x58, 0x6e, 0x9d, 0x69, 0xcd, 0xa4, 0xde, 0xcc, 0x85,
	0x47, 0x65, 0x2b, 0xb1, 0x21, 0x24, 0x0d, 0x22, 0x99, 0x2c, 0x1e, 0x75, 0xc6, 0xe9, 0xb4, 0x1f,
	0x0c, 0x23, 0x52, 0x19, 0x78, 0x71, 0x03, 0xd9, 0x99, 0xa6, 0x2b, 0x4e, 0xe2, 0x9d, 0x12, 0xb3,
	0x6b, 0xac, 0x9c, 0xc5, 0x5d, 0xf3, 0x1e, 0x42, 0xb7, 0xaa, 0xc5, 0x42, 0x8a, 0x36, 0x66, 0x52,
	0xd5, 0x62, 0x2e, 0xd8, 0x10, 0xd2, 0x15, 0x5e, 0x65, 0x46, 0x65, 0x7d, 0xbc, 0x7e, 0x09, 0x55,
	0x2d, 0x4e, 0x03, 0x29, 0xfe, 0x44, 0x30, 0xd8, 0x70, 0x7c, 0x97, 0x7c, 0xfb, 0x10, 0xaf, 0xcf,
	0xc6, 0x52, 0xb0, 0xe7, 0xf0, 0x10, 0xfd, 0x1e, 0x4d, 0xfe, 0xe0, 0x9d, 0xc8, 0x6b, 0x89, 0x3d,
	0x85, 0x9e, 0xb1, 0x62, 0xa1, 0x9d, 0xcd, 0x1e, 0xf8, 0xd9, 0xae, 0xb1, 0xe2, 0x83, 0xb3, 0xb7,
	0x02, 0x12, 0x65, 0xc9, 0x5a, 0x98, 0x11, 0x15, 0x6f, 0xe1, 0xc9, 0xe9, 0x12, 0x39, 0xfd, 0x67,
	0x27, 0x8a, 0x37, 0x70, 0xf8, 0xcf, 0xfc, 0x0e, 0x31, 0xa7, 0xbf, 0x62, 0x38, 0xb8, 0xf8, 0xc2,
	0x29, 0xb4, 0xe0, 0x7c, 0x55, 0x62, 0xf6, 0x11, 0xd8, 0xfd, 0x76, 0xb0, 0x63, 0x3f, 0xbe, 0xb5,
	0xc3, 0xf9, 0x70, 0xab, 0xde, 0xfa, 0x29, 0xe1, 0xf1, 0xbd, 0x7f, 0xc2, 0x8e, 0xfc, 0xd4, 0xb6,
	0xa2, 0xe4, 0xc7, 0xdb, 0xe4, 0x76, 0xe7, 0x19, 0x3c, 0xba, 0x13, 0x9e, 0x0d, 0xfc, 0xc0, 0xa6,
	0x0f, 0x9a, 0xe7, 0x9b, 0xa4, 0xb0, 0xe7, 0xa4, 0xf7, 0x29, 0x99, 0xbc, 0xe2, 0x8d, 0xbc, 0xec,
	0xfa, 0xe7, 0xfa, 0xfa, 0x6f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x43, 0x67, 0xa2, 0xfe, 0xd0, 0x03,
	0x00, 0x00,
}
