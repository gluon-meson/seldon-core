/*
Copyright 2022 Seldon Technologies Ltd.

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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.9
// source: mlops/proxy/proxy.proto

package proxy

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// SchedulerProxyClient is the client API for SchedulerProxy service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type SchedulerProxyClient interface {
	LoadModel(ctx context.Context, in *LoadModelRequest, opts ...grpc.CallOption) (*LoadModelResponse, error)
	UnloadModel(ctx context.Context, in *UnloadModelRequest, opts ...grpc.CallOption) (*UnloadModelResponse, error)
}

type schedulerProxyClient struct {
	cc grpc.ClientConnInterface
}

func NewSchedulerProxyClient(cc grpc.ClientConnInterface) SchedulerProxyClient {
	return &schedulerProxyClient{cc}
}

func (c *schedulerProxyClient) LoadModel(ctx context.Context, in *LoadModelRequest, opts ...grpc.CallOption) (*LoadModelResponse, error) {
	out := new(LoadModelResponse)
	err := c.cc.Invoke(ctx, "/seldon.mlops.proxy.SchedulerProxy/LoadModel", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerProxyClient) UnloadModel(ctx context.Context, in *UnloadModelRequest, opts ...grpc.CallOption) (*UnloadModelResponse, error) {
	out := new(UnloadModelResponse)
	err := c.cc.Invoke(ctx, "/seldon.mlops.proxy.SchedulerProxy/UnloadModel", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SchedulerProxyServer is the server API for SchedulerProxy service.
// All implementations must embed UnimplementedSchedulerProxyServer
// for forward compatibility
type SchedulerProxyServer interface {
	LoadModel(context.Context, *LoadModelRequest) (*LoadModelResponse, error)
	UnloadModel(context.Context, *UnloadModelRequest) (*UnloadModelResponse, error)
	mustEmbedUnimplementedSchedulerProxyServer()
}

// UnimplementedSchedulerProxyServer must be embedded to have forward compatible implementations.
type UnimplementedSchedulerProxyServer struct {
}

func (UnimplementedSchedulerProxyServer) LoadModel(context.Context, *LoadModelRequest) (*LoadModelResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LoadModel not implemented")
}
func (UnimplementedSchedulerProxyServer) UnloadModel(context.Context, *UnloadModelRequest) (*UnloadModelResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UnloadModel not implemented")
}
func (UnimplementedSchedulerProxyServer) mustEmbedUnimplementedSchedulerProxyServer() {}

// UnsafeSchedulerProxyServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to SchedulerProxyServer will
// result in compilation errors.
type UnsafeSchedulerProxyServer interface {
	mustEmbedUnimplementedSchedulerProxyServer()
}

func RegisterSchedulerProxyServer(s grpc.ServiceRegistrar, srv SchedulerProxyServer) {
	s.RegisterService(&SchedulerProxy_ServiceDesc, srv)
}

func _SchedulerProxy_LoadModel_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LoadModelRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerProxyServer).LoadModel(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/seldon.mlops.proxy.SchedulerProxy/LoadModel",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerProxyServer).LoadModel(ctx, req.(*LoadModelRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SchedulerProxy_UnloadModel_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UnloadModelRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerProxyServer).UnloadModel(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/seldon.mlops.proxy.SchedulerProxy/UnloadModel",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerProxyServer).UnloadModel(ctx, req.(*UnloadModelRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// SchedulerProxy_ServiceDesc is the grpc.ServiceDesc for SchedulerProxy service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var SchedulerProxy_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "seldon.mlops.proxy.SchedulerProxy",
	HandlerType: (*SchedulerProxyServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "LoadModel",
			Handler:    _SchedulerProxy_LoadModel_Handler,
		},
		{
			MethodName: "UnloadModel",
			Handler:    _SchedulerProxy_UnloadModel_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "mlops/proxy/proxy.proto",
}
