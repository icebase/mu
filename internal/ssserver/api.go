package ssserver

import (
	"context"
	"errors"
	"log/slog"
	"net"

	"github.com/icebase/mu/pkg/ss"
	pb "github.com/icebase/mu/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type managerService struct {
	pb.UnimplementedSSManagerServiceServer
	server *Server
}

// NewAPIServer builds the authenticated management gRPC server.
func NewAPIServer(server *Server, token string) *grpc.Server {
	api := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get(ss.TokenMetadataKey)
		if len(values) != 1 || !tokenEqual(values[0], token) {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		return handler(ctx, req)
	}))
	pb.RegisterSSManagerServiceServer(api, &managerService{server: server})
	return api
}

// ServeAPI serves until the listener closes.
func ServeAPI(api *grpc.Server, listener net.Listener) error {
	return api.Serve(listener)
}

func (m *managerService) ListUsers(context.Context, *pb.SSListUsersRequest) (*pb.SSListUsersResponse, error) {
	users := m.server.Users()
	resp := &pb.SSListUsersResponse{Users: make([]*pb.SSServerUser, 0, len(users))}
	for _, u := range users {
		resp.Users = append(resp.Users, &pb.SSServerUser{Name: u.Name, PskFingerprint: u.PSKFingerprint, Upload: u.Upload, Download: u.Download})
	}
	return resp, nil
}

func (m *managerService) SetUser(_ context.Context, req *pb.SSSetUserRequest) (*pb.SSSetUserResponse, error) {
	if err := m.server.SetUser(req.GetName(), req.GetPsk()); err != nil {
		switch {
		case errors.Is(err, ErrInvalidUser):
			return nil, status.Error(codes.InvalidArgument, "invalid user name or key")
		case errors.Is(err, ErrKeyInUse):
			return nil, status.Error(codes.AlreadyExists, "key already belongs to another user")
		default:
			slog.Error("set user failed", "error", err)
			return nil, status.Error(codes.Internal, "set user failed")
		}
	}
	return &pb.SSSetUserResponse{}, nil
}

func (m *managerService) RemoveUser(_ context.Context, req *pb.SSRemoveUserRequest) (*pb.SSRemoveUserResponse, error) {
	if err := m.server.RemoveUser(req.GetName()); err != nil {
		slog.Error("remove user failed", "error", err)
		return nil, status.Error(codes.Internal, "remove user failed")
	}
	return &pb.SSRemoveUserResponse{}, nil
}
