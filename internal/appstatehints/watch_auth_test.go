package appstatehints

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	client "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type expiringWatchServer struct {
	pb.UnimplementedAuthServer
	pb.UnimplementedKVServer
	pb.UnimplementedWatchServer
	epoch   atomic.Int64
	streams atomic.Int64
	active  atomic.Int64
	ended   chan struct{}
}

func (s *expiringWatchServer) Authenticate(context.Context, *pb.AuthenticateRequest) (*pb.AuthenticateResponse, error) {
	return &pb.AuthenticateResponse{Token: fmt.Sprint(s.epoch.Load())}, nil
}
func watchTestToken(ctx context.Context) string {
	m, _ := metadata.FromIncomingContext(ctx)
	v := m.Get("token")
	if len(v) > 0 {
		return v[0]
	}
	return ""
}
func (s *expiringWatchServer) Range(ctx context.Context, _ *pb.RangeRequest) (*pb.RangeResponse, error) {
	if watchTestToken(ctx) != fmt.Sprint(s.epoch.Load()) {
		return nil, rpctypes.ErrGRPCInvalidAuthToken
	}
	return &pb.RangeResponse{Header: &pb.ResponseHeader{Revision: 1}}, nil
}
func (s *expiringWatchServer) Watch(stream pb.Watch_WatchServer) error {
	s.streams.Add(1)
	s.active.Add(1)
	defer func() { s.active.Add(-1); s.ended <- struct{}{} }()
	captured := watchTestToken(stream.Context())
	var id int64
	for {
		r, e := stream.Recv()
		if e != nil {
			return e
		}
		if r.GetCreateRequest() == nil {
			continue
		}
		id++
		v := &pb.WatchResponse{Header: &pb.ResponseHeader{Revision: 1}, WatchId: id, Created: true}
		if captured != fmt.Sprint(s.epoch.Load()) {
			v.Canceled = true
			v.CancelReason = "etcdserver: invalid auth token"
		}
		if e = stream.Send(v); e != nil {
			return e
		}
		if !v.Canceled {
			if e = stream.Send(&pb.WatchResponse{Header: &pb.ResponseHeader{Revision: 2}, WatchId: id, Events: []*mvccpb.Event{{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Key: r.GetCreateRequest().Key, ModRevision: 2, Value: []byte(`{"schemaVersion":1,"revision":"renewed:2","changedAt":"2026-11-04T00:00:00Z"}`)}}}}); e != nil {
				return e
			}
		}
	}
}
func receiveTestWatch(ctx context.Context, ch client.WatchChan) client.WatchResponse {
	select {
	case r := <-ch:
		return r
	case <-ctx.Done():
		panic("watch response timed out")
	}
}

func TestSubscriptionAfterTokenRenewalDoesNotReusePinnedStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lis, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	srv := grpc.NewServer()
	f := &expiringWatchServer{ended: make(chan struct{}, 8)}
	f.epoch.Store(1)
	pb.RegisterAuthServer(srv, f)
	pb.RegisterKVServer(srv, f)
	pb.RegisterWatchServer(srv, f)
	go srv.Serve(lis)
	defer srv.Stop()
	c, e := client.New(client.Config{Endpoints: []string{lis.Addr().String()}, Username: "test", Password: "test", DialTimeout: time.Second, Logger: zap.NewNop()})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	pin := c.Watch(ctx, "pin", client.WithCreatedNotify())
	if r := receiveTestWatch(ctx, pin); !r.Created || r.Canceled {
		t.Fatal("first watch failed")
	}
	f.epoch.Store(2)
	subCtx, stop := context.WithCancel(ctx)
	defer stop()
	events, e := Subscribe(subCtx, c, "00000000-0000-7000-8000-000000000001", "00000000-0000-7000-8000-000000000002")
	if e != nil {
		t.Fatal(e)
	}
	for {
		select {
		case v, ok := <-events:
			if !ok {
				t.Fatal("subscription closed after successful authentication refresh")
			}
			if v.Revision == "renewed:2" {
				stop()
				select {
				case _, open := <-events:
					if open {
						t.Fatal("subscription survived cancellation")
					}
				case <-ctx.Done():
					t.Fatal("cancel did not close subscription")
				}
				select {
				case <-f.ended:
				case <-ctx.Done():
					t.Fatal("owned stream leaked")
				}
				if f.active.Load() != 1 {
					t.Fatal("cancel affected the unrelated pinned watch")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("fresh subscription did not receive event")
		}
	}
}
