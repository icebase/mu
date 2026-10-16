package app

import (
	"context"
	"maps"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/api/service"
	"google.golang.org/grpc"
)

func TestTrojanSyncReconcilesUsers(t *testing.T) {
	enabled := &pb.User{Enable: 1, V2RayUser: &pb.VUser{Uuid: "enabled"}}
	disabled := &pb.User{Enable: 0, V2RayUser: &pb.VUser{Uuid: "disabled"}}
	newUser := &pb.User{Enable: 1, V2RayUser: &pb.VUser{Uuid: "new"}}

	tests := []struct {
		name       string
		initial    []string
		users      []*pb.User
		want       []string
		wantAdds   int
		wantDelete int
	}{
		{
			name:    "keep enabled user across syncs",
			initial: []string{"enabled"},
			users:   []*pb.User{enabled},
			want:    []string{"enabled"},
		},
		{
			name:       "remove disabled user",
			initial:    []string{"disabled"},
			users:      []*pb.User{disabled},
			wantDelete: 1,
		},
		{
			name:       "remove user missing from mu",
			initial:    []string{"removed"},
			users:      []*pb.User{enabled},
			want:       []string{"enabled"},
			wantAdds:   1,
			wantDelete: 1,
		},
		{
			name:     "add enabled user only once",
			users:    []*pb.User{enabled},
			want:     []string{"enabled"},
			wantAdds: 1,
		},
		{
			name:  "do not add disabled user",
			users: []*pb.User{disabled},
		},
		{
			name:       "empty mu list removes all users",
			initial:    []string{"enabled", "removed"},
			wantDelete: 2,
		},
		{
			name:       "reconcile mixed users",
			initial:    []string{"enabled", "disabled", "removed"},
			users:      []*pb.User{enabled, disabled, newUser},
			want:       []string{"enabled", "new"},
			wantAdds:   1,
			wantDelete: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &trojanTestServer{users: make(map[string]bool)}
			for _, password := range tt.initial {
				backend.users[password] = true
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			service.RegisterTrojanServerServiceServer(server, backend)
			t.Cleanup(server.Stop)
			go func() {
				_ = server.Serve(listener)
			}()

			syncer, err := newTrojanSync(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Repeated calls model successive timer ticks without waiting 60 seconds.
			for round := range 3 {
				if err := syncer.Sync(ctx, tt.users); err != nil {
					t.Fatalf("sync %d: %v", round+1, err)
				}
				backend.mu.Lock()
				got := slices.Sorted(maps.Keys(backend.users))
				adds, deletes := backend.adds, backend.deletes
				backend.mu.Unlock()

				if !slices.Equal(got, tt.want) {
					t.Errorf("sync %d: users = %v, want %v", round+1, got, tt.want)
				}
				if adds != tt.wantAdds || deletes != tt.wantDelete {
					t.Errorf("sync %d: adds/deletes = %d/%d, want %d/%d",
						round+1, adds, deletes, tt.wantAdds, tt.wantDelete)
				}
			}
		})
	}
}

// Exercise Sync through the real Trojan manager and an isolated gRPC backend.
// The backend returns a snapshot and records mutations just like the Trojan API.
type trojanTestServer struct {
	service.UnimplementedTrojanServerServiceServer
	mu      sync.Mutex
	users   map[string]bool
	adds    int
	deletes int
}

func (s *trojanTestServer) ListUsers(_ *service.ListUsersRequest, stream service.TrojanServerService_ListUsersServer) error {
	s.mu.Lock()
	passwords := slices.Sorted(maps.Keys(s.users))
	s.mu.Unlock()
	for _, password := range passwords {
		if err := stream.Send(&service.ListUsersResponse{
			Status: &service.UserStatus{User: &service.User{Password: password}},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *trojanTestServer) SetUsers(stream service.TrojanServerService_SetUsersServer) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	s.mu.Lock()
	password := request.Status.User.Password
	switch request.Operation {
	case service.SetUsersRequest_Add:
		s.users[password] = true
		s.adds++
	case service.SetUsersRequest_Delete:
		delete(s.users, password)
		s.deletes++
	}
	s.mu.Unlock()
	return stream.Send(&service.SetUsersResponse{Success: true})
}
