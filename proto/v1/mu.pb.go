// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.1
// 	protoc        (unknown)
// source: v1/mu.proto

package v1

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

type TrafficType int32

const (
	TrafficType_TRAFFIC_TYPE_UNSPECIFIED TrafficType = 0
	TrafficType_TRAFFIC_TYPE_V2FLY       TrafficType = 1
	TrafficType_TRAFFIC_TYPE_TROJAN      TrafficType = 2
)

// Enum value maps for TrafficType.
var (
	TrafficType_name = map[int32]string{
		0: "TRAFFIC_TYPE_UNSPECIFIED",
		1: "TRAFFIC_TYPE_V2FLY",
		2: "TRAFFIC_TYPE_TROJAN",
	}
	TrafficType_value = map[string]int32{
		"TRAFFIC_TYPE_UNSPECIFIED": 0,
		"TRAFFIC_TYPE_V2FLY":       1,
		"TRAFFIC_TYPE_TROJAN":      2,
	}
)

func (x TrafficType) Enum() *TrafficType {
	p := new(TrafficType)
	*p = x
	return p
}

func (x TrafficType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TrafficType) Descriptor() protoreflect.EnumDescriptor {
	return file_v1_mu_proto_enumTypes[0].Descriptor()
}

func (TrafficType) Type() protoreflect.EnumType {
	return &file_v1_mu_proto_enumTypes[0]
}

func (x TrafficType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TrafficType.Descriptor instead.
func (TrafficType) EnumDescriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{0}
}

type GetUsersRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeId string `protobuf:"bytes,1,opt,name=node_id,json=nodeId,proto3" json:"node_id,omitempty"`
}

func (x *GetUsersRequest) Reset() {
	*x = GetUsersRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetUsersRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetUsersRequest) ProtoMessage() {}

func (x *GetUsersRequest) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetUsersRequest.ProtoReflect.Descriptor instead.
func (*GetUsersRequest) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{0}
}

func (x *GetUsersRequest) GetNodeId() string {
	if x != nil {
		return x.NodeId
	}
	return ""
}

type GetUsersResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Users []*User `protobuf:"bytes,1,rep,name=users,proto3" json:"users,omitempty"`
}

func (x *GetUsersResponse) Reset() {
	*x = GetUsersResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetUsersResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetUsersResponse) ProtoMessage() {}

func (x *GetUsersResponse) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetUsersResponse.ProtoReflect.Descriptor instead.
func (*GetUsersResponse) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{1}
}

func (x *GetUsersResponse) GetUsers() []*User {
	if x != nil {
		return x.Users
	}
	return nil
}

type UploadTrafficLogRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeId   string            `protobuf:"bytes,1,opt,name=node_id,json=nodeId,proto3" json:"node_id,omitempty"`
	Logs     []*UserTrafficLog `protobuf:"bytes,2,rep,name=logs,proto3" json:"logs,omitempty"`
	UploadAt int64             `protobuf:"varint,3,opt,name=upload_at,json=uploadAt,proto3" json:"upload_at,omitempty"`
}

func (x *UploadTrafficLogRequest) Reset() {
	*x = UploadTrafficLogRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UploadTrafficLogRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UploadTrafficLogRequest) ProtoMessage() {}

func (x *UploadTrafficLogRequest) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UploadTrafficLogRequest.ProtoReflect.Descriptor instead.
func (*UploadTrafficLogRequest) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{2}
}

func (x *UploadTrafficLogRequest) GetNodeId() string {
	if x != nil {
		return x.NodeId
	}
	return ""
}

func (x *UploadTrafficLogRequest) GetLogs() []*UserTrafficLog {
	if x != nil {
		return x.Logs
	}
	return nil
}

func (x *UploadTrafficLogRequest) GetUploadAt() int64 {
	if x != nil {
		return x.UploadAt
	}
	return 0
}

type UploadTrafficLogResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *UploadTrafficLogResponse) Reset() {
	*x = UploadTrafficLogResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UploadTrafficLogResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UploadTrafficLogResponse) ProtoMessage() {}

func (x *UploadTrafficLogResponse) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UploadTrafficLogResponse.ProtoReflect.Descriptor instead.
func (*UploadTrafficLogResponse) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{3}
}

type VUser struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Email   string `protobuf:"bytes,1,opt,name=email,proto3" json:"email,omitempty"`
	Uuid    string `protobuf:"bytes,2,opt,name=uuid,proto3" json:"uuid,omitempty"`
	AlterId uint32 `protobuf:"varint,3,opt,name=alter_id,json=alterId,proto3" json:"alter_id,omitempty"`
	Level   uint32 `protobuf:"varint,4,opt,name=level,proto3" json:"level,omitempty"`
}

func (x *VUser) Reset() {
	*x = VUser{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *VUser) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*VUser) ProtoMessage() {}

func (x *VUser) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use VUser.ProtoReflect.Descriptor instead.
func (*VUser) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{4}
}

func (x *VUser) GetEmail() string {
	if x != nil {
		return x.Email
	}
	return ""
}

func (x *VUser) GetUuid() string {
	if x != nil {
		return x.Uuid
	}
	return ""
}

func (x *VUser) GetAlterId() uint32 {
	if x != nil {
		return x.AlterId
	}
	return 0
}

func (x *VUser) GetLevel() uint32 {
	if x != nil {
		return x.Level
	}
	return 0
}

type User struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id             int64  `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Port           int32  `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
	Passwd         string `protobuf:"bytes,3,opt,name=passwd,proto3" json:"passwd,omitempty"`
	Method         string `protobuf:"bytes,4,opt,name=method,proto3" json:"method,omitempty"`
	Enable         int32  `protobuf:"varint,5,opt,name=enable,proto3" json:"enable,omitempty"`
	TransferEnable int64  `protobuf:"varint,6,opt,name=transfer_enable,json=transferEnable,proto3" json:"transfer_enable,omitempty"`
	U              int64  `protobuf:"varint,7,opt,name=u,proto3" json:"u,omitempty"`
	D              int64  `protobuf:"varint,8,opt,name=d,proto3" json:"d,omitempty"`
	V2RayUser      *VUser `protobuf:"bytes,9,opt,name=v2ray_user,json=v2rayUser,proto3" json:"v2ray_user,omitempty"`
}

func (x *User) Reset() {
	*x = User{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *User) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*User) ProtoMessage() {}

func (x *User) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use User.ProtoReflect.Descriptor instead.
func (*User) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{5}
}

func (x *User) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *User) GetPort() int32 {
	if x != nil {
		return x.Port
	}
	return 0
}

func (x *User) GetPasswd() string {
	if x != nil {
		return x.Passwd
	}
	return ""
}

func (x *User) GetMethod() string {
	if x != nil {
		return x.Method
	}
	return ""
}

func (x *User) GetEnable() int32 {
	if x != nil {
		return x.Enable
	}
	return 0
}

func (x *User) GetTransferEnable() int64 {
	if x != nil {
		return x.TransferEnable
	}
	return 0
}

func (x *User) GetU() int64 {
	if x != nil {
		return x.U
	}
	return 0
}

func (x *User) GetD() int64 {
	if x != nil {
		return x.D
	}
	return 0
}

func (x *User) GetV2RayUser() *VUser {
	if x != nil {
		return x.V2RayUser
	}
	return nil
}

type UserTrafficLog struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	UserId      int64  `protobuf:"varint,1,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	Uuid        string `protobuf:"bytes,2,opt,name=uuid,proto3" json:"uuid,omitempty"`
	U           int64  `protobuf:"varint,3,opt,name=u,proto3" json:"u,omitempty"`
	D           int64  `protobuf:"varint,4,opt,name=d,proto3" json:"d,omitempty"`
	TrafficType int32  `protobuf:"varint,5,opt,name=traffic_type,json=trafficType,proto3" json:"traffic_type,omitempty"`
}

func (x *UserTrafficLog) Reset() {
	*x = UserTrafficLog{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UserTrafficLog) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UserTrafficLog) ProtoMessage() {}

func (x *UserTrafficLog) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UserTrafficLog.ProtoReflect.Descriptor instead.
func (*UserTrafficLog) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{6}
}

func (x *UserTrafficLog) GetUserId() int64 {
	if x != nil {
		return x.UserId
	}
	return 0
}

func (x *UserTrafficLog) GetUuid() string {
	if x != nil {
		return x.Uuid
	}
	return ""
}

func (x *UserTrafficLog) GetU() int64 {
	if x != nil {
		return x.U
	}
	return 0
}

func (x *UserTrafficLog) GetD() int64 {
	if x != nil {
		return x.D
	}
	return 0
}

func (x *UserTrafficLog) GetTrafficType() int32 {
	if x != nil {
		return x.TrafficType
	}
	return 0
}

type PingRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeId  string `protobuf:"bytes,1,opt,name=node_id,json=nodeId,proto3" json:"node_id,omitempty"`
	Version string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *PingRequest) Reset() {
	*x = PingRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingRequest) ProtoMessage() {}

func (x *PingRequest) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingRequest.ProtoReflect.Descriptor instead.
func (*PingRequest) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{7}
}

func (x *PingRequest) GetNodeId() string {
	if x != nil {
		return x.NodeId
	}
	return ""
}

func (x *PingRequest) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

type PingResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingResponse) Reset() {
	*x = PingResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1_mu_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingResponse) ProtoMessage() {}

func (x *PingResponse) ProtoReflect() protoreflect.Message {
	mi := &file_v1_mu_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingResponse.ProtoReflect.Descriptor instead.
func (*PingResponse) Descriptor() ([]byte, []int) {
	return file_v1_mu_proto_rawDescGZIP(), []int{8}
}

var File_v1_mu_proto protoreflect.FileDescriptor

var file_v1_mu_proto_rawDesc = []byte{
	0x0a, 0x0b, 0x76, 0x31, 0x2f, 0x6d, 0x75, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x76,
	0x31, 0x22, 0x2a, 0x0a, 0x0f, 0x47, 0x65, 0x74, 0x55, 0x73, 0x65, 0x72, 0x73, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x17, 0x0a, 0x07, 0x6e, 0x6f, 0x64, 0x65, 0x5f, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x6e, 0x6f, 0x64, 0x65, 0x49, 0x64, 0x22, 0x32, 0x0a,
	0x10, 0x47, 0x65, 0x74, 0x55, 0x73, 0x65, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x1e, 0x0a, 0x05, 0x75, 0x73, 0x65, 0x72, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x08, 0x2e, 0x76, 0x31, 0x2e, 0x55, 0x73, 0x65, 0x72, 0x52, 0x05, 0x75, 0x73, 0x65, 0x72,
	0x73, 0x22, 0x77, 0x0a, 0x17, 0x55, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x54, 0x72, 0x61, 0x66, 0x66,
	0x69, 0x63, 0x4c, 0x6f, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x17, 0x0a, 0x07,
	0x6e, 0x6f, 0x64, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x6e,
	0x6f, 0x64, 0x65, 0x49, 0x64, 0x12, 0x26, 0x0a, 0x04, 0x6c, 0x6f, 0x67, 0x73, 0x18, 0x02, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x76, 0x31, 0x2e, 0x55, 0x73, 0x65, 0x72, 0x54, 0x72, 0x61,
	0x66, 0x66, 0x69, 0x63, 0x4c, 0x6f, 0x67, 0x52, 0x04, 0x6c, 0x6f, 0x67, 0x73, 0x12, 0x1b, 0x0a,
	0x09, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x5f, 0x61, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03,
	0x52, 0x08, 0x75, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x41, 0x74, 0x22, 0x1a, 0x0a, 0x18, 0x55, 0x70,
	0x6c, 0x6f, 0x61, 0x64, 0x54, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x4c, 0x6f, 0x67, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x62, 0x0a, 0x05, 0x56, 0x55, 0x73, 0x65, 0x72, 0x12,
	0x14, 0x0a, 0x05, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05,
	0x65, 0x6d, 0x61, 0x69, 0x6c, 0x12, 0x12, 0x0a, 0x04, 0x75, 0x75, 0x69, 0x64, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x75, 0x75, 0x69, 0x64, 0x12, 0x19, 0x0a, 0x08, 0x61, 0x6c, 0x74,
	0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x07, 0x61, 0x6c, 0x74,
	0x65, 0x72, 0x49, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x6c, 0x65, 0x76, 0x65, 0x6c, 0x18, 0x04, 0x20,
	0x01, 0x28, 0x0d, 0x52, 0x05, 0x6c, 0x65, 0x76, 0x65, 0x6c, 0x22, 0xe1, 0x01, 0x0a, 0x04, 0x55,
	0x73, 0x65, 0x72, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x02, 0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x61, 0x73, 0x73, 0x77,
	0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x70, 0x61, 0x73, 0x73, 0x77, 0x64, 0x12,
	0x16, 0x0a, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x16, 0x0a, 0x06, 0x65, 0x6e, 0x61, 0x62, 0x6c,
	0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x06, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x12,
	0x27, 0x0a, 0x0f, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72, 0x5f, 0x65, 0x6e, 0x61, 0x62,
	0x6c, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0e, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x66,
	0x65, 0x72, 0x45, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x12, 0x0c, 0x0a, 0x01, 0x75, 0x18, 0x07, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x01, 0x75, 0x12, 0x0c, 0x0a, 0x01, 0x64, 0x18, 0x08, 0x20, 0x01, 0x28,
	0x03, 0x52, 0x01, 0x64, 0x12, 0x28, 0x0a, 0x0a, 0x76, 0x32, 0x72, 0x61, 0x79, 0x5f, 0x75, 0x73,
	0x65, 0x72, 0x18, 0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x76, 0x31, 0x2e, 0x56, 0x55,
	0x73, 0x65, 0x72, 0x52, 0x09, 0x76, 0x32, 0x72, 0x61, 0x79, 0x55, 0x73, 0x65, 0x72, 0x22, 0x7c,
	0x0a, 0x0e, 0x55, 0x73, 0x65, 0x72, 0x54, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x4c, 0x6f, 0x67,
	0x12, 0x17, 0x0a, 0x07, 0x75, 0x73, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x03, 0x52, 0x06, 0x75, 0x73, 0x65, 0x72, 0x49, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x75, 0x75, 0x69,
	0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x75, 0x75, 0x69, 0x64, 0x12, 0x0c, 0x0a,
	0x01, 0x75, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x01, 0x75, 0x12, 0x0c, 0x0a, 0x01, 0x64,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x01, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x74, 0x72, 0x61,
	0x66, 0x66, 0x69, 0x63, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x0b, 0x74, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x54, 0x79, 0x70, 0x65, 0x22, 0x40, 0x0a, 0x0b,
	0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x17, 0x0a, 0x07, 0x6e,
	0x6f, 0x64, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x6e, 0x6f,
	0x64, 0x65, 0x49, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x0e,
	0x0a, 0x0c, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2a, 0x5c,
	0x0a, 0x0b, 0x54, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1c, 0x0a,
	0x18, 0x54, 0x52, 0x41, 0x46, 0x46, 0x49, 0x43, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x55, 0x4e,
	0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x16, 0x0a, 0x12, 0x54,
	0x52, 0x41, 0x46, 0x46, 0x49, 0x43, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x56, 0x32, 0x46, 0x4c,
	0x59, 0x10, 0x01, 0x12, 0x17, 0x0a, 0x13, 0x54, 0x52, 0x41, 0x46, 0x46, 0x49, 0x43, 0x5f, 0x54,
	0x59, 0x50, 0x45, 0x5f, 0x54, 0x52, 0x4f, 0x4a, 0x41, 0x4e, 0x10, 0x02, 0x32, 0xc2, 0x01, 0x0a,
	0x09, 0x4d, 0x55, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x2b, 0x0a, 0x04, 0x50, 0x69,
	0x6e, 0x67, 0x12, 0x0f, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x10, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x37, 0x0a, 0x08, 0x47, 0x65, 0x74, 0x55, 0x73,
	0x65, 0x72, 0x73, 0x12, 0x13, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x55, 0x73, 0x65, 0x72,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x14, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65,
	0x74, 0x55, 0x73, 0x65, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00,
	0x12, 0x4f, 0x0a, 0x10, 0x55, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x54, 0x72, 0x61, 0x66, 0x66, 0x69,
	0x63, 0x4c, 0x6f, 0x67, 0x12, 0x1b, 0x2e, 0x76, 0x31, 0x2e, 0x55, 0x70, 0x6c, 0x6f, 0x61, 0x64,
	0x54, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x4c, 0x6f, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x1c, 0x2e, 0x76, 0x31, 0x2e, 0x55, 0x70, 0x6c, 0x6f, 0x61, 0x64, 0x54, 0x72, 0x61,
	0x66, 0x66, 0x69, 0x63, 0x4c, 0x6f, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x00, 0x42, 0x20, 0x5a, 0x1e, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f,
	0x69, 0x63, 0x65, 0x62, 0x61, 0x73, 0x65, 0x2f, 0x6d, 0x75, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_v1_mu_proto_rawDescOnce sync.Once
	file_v1_mu_proto_rawDescData = file_v1_mu_proto_rawDesc
)

func file_v1_mu_proto_rawDescGZIP() []byte {
	file_v1_mu_proto_rawDescOnce.Do(func() {
		file_v1_mu_proto_rawDescData = protoimpl.X.CompressGZIP(file_v1_mu_proto_rawDescData)
	})
	return file_v1_mu_proto_rawDescData
}

var file_v1_mu_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_v1_mu_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_v1_mu_proto_goTypes = []interface{}{
	(TrafficType)(0),                 // 0: v1.TrafficType
	(*GetUsersRequest)(nil),          // 1: v1.GetUsersRequest
	(*GetUsersResponse)(nil),         // 2: v1.GetUsersResponse
	(*UploadTrafficLogRequest)(nil),  // 3: v1.UploadTrafficLogRequest
	(*UploadTrafficLogResponse)(nil), // 4: v1.UploadTrafficLogResponse
	(*VUser)(nil),                    // 5: v1.VUser
	(*User)(nil),                     // 6: v1.User
	(*UserTrafficLog)(nil),           // 7: v1.UserTrafficLog
	(*PingRequest)(nil),              // 8: v1.PingRequest
	(*PingResponse)(nil),             // 9: v1.PingResponse
}
var file_v1_mu_proto_depIdxs = []int32{
	6, // 0: v1.GetUsersResponse.users:type_name -> v1.User
	7, // 1: v1.UploadTrafficLogRequest.logs:type_name -> v1.UserTrafficLog
	5, // 2: v1.User.v2ray_user:type_name -> v1.VUser
	8, // 3: v1.MUService.Ping:input_type -> v1.PingRequest
	1, // 4: v1.MUService.GetUsers:input_type -> v1.GetUsersRequest
	3, // 5: v1.MUService.UploadTrafficLog:input_type -> v1.UploadTrafficLogRequest
	9, // 6: v1.MUService.Ping:output_type -> v1.PingResponse
	2, // 7: v1.MUService.GetUsers:output_type -> v1.GetUsersResponse
	4, // 8: v1.MUService.UploadTrafficLog:output_type -> v1.UploadTrafficLogResponse
	6, // [6:9] is the sub-list for method output_type
	3, // [3:6] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_v1_mu_proto_init() }
func file_v1_mu_proto_init() {
	if File_v1_mu_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_v1_mu_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetUsersRequest); i {
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
		file_v1_mu_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetUsersResponse); i {
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
		file_v1_mu_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UploadTrafficLogRequest); i {
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
		file_v1_mu_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UploadTrafficLogResponse); i {
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
		file_v1_mu_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*VUser); i {
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
		file_v1_mu_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*User); i {
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
		file_v1_mu_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UserTrafficLog); i {
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
		file_v1_mu_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingRequest); i {
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
		file_v1_mu_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingResponse); i {
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
			RawDescriptor: file_v1_mu_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_v1_mu_proto_goTypes,
		DependencyIndexes: file_v1_mu_proto_depIdxs,
		EnumInfos:         file_v1_mu_proto_enumTypes,
		MessageInfos:      file_v1_mu_proto_msgTypes,
	}.Build()
	File_v1_mu_proto = out.File
	file_v1_mu_proto_rawDesc = nil
	file_v1_mu_proto_goTypes = nil
	file_v1_mu_proto_depIdxs = nil
}
