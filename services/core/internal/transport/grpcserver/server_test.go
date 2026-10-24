package grpcserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type uploadProbe struct {
	pb.UnimplementedImportsServiceServer
	upload func(grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error
}

func (p *uploadProbe) UploadImport(stream grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
	return p.upload(stream)
}

func TestUploadAllowsAPIAndOneMiBPayloadWithBoundedContext(t *testing.T) {
	chunk := &pb.UploadImportRequest{Frame: &pb.UploadImportRequest_Chunk{Chunk: bytes.Repeat([]byte{42}, 1<<20)}}
	if proto.Size(chunk) <= 1<<20 {
		t.Fatal("test payload must exceed the old frame limit after protobuf encoding")
	}
	observed := make(chan time.Duration, 1)
	probe := &uploadProbe{upload: func(stream grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
		deadline, ok := stream.Context().Deadline()
		if !ok || mtls.Identity(stream.Context()) != mtls.API {
			return status.Error(codes.Internal, "missing bounded authenticated context")
		}
		observed <- time.Until(deadline)
		metadata, err := stream.Recv()
		if err != nil {
			return err
		}
		if metadata.GetMetadata() == nil {
			return status.Error(codes.InvalidArgument, "metadata required")
		}
		received, err := stream.Recv()
		if err != nil {
			return err
		}
		if !bytes.Equal(received.GetChunk(), chunk.GetChunk()) {
			return status.Error(codes.DataLoss, "chunk changed")
		}
		if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
			return status.Error(codes.InvalidArgument, "expected end of stream")
		}
		return stream.SendAndClose(&pb.UploadImportResponse{Accepted: &pb.ImportAccepted{Id: "transport-proof", Status: pb.ImportStatus_IMPORT_STATUS_QUEUED}})
	}}
	address, ca := startTransportServer(t, probe)
	conn := transportClient(t, address, ca, mtls.API)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := pb.NewImportsServiceClient(conn).UploadImport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&pb.UploadImportRequest{Frame: &pb.UploadImportRequest_Metadata{Metadata: &pb.UploadImportMetadata{ByteSize: 1 << 20}}}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(chunk); err != nil {
		t.Fatal(err)
	}
	response, err := stream.CloseAndRecv()
	if err != nil || response.GetAccepted().GetId() != "transport-proof" {
		t.Fatal("API upload failed", err)
	}
	if remaining := <-observed; remaining <= 0 || remaining > 2*time.Minute {
		t.Fatal("server did not impose its two-minute stream budget", remaining)
	}
}

func TestUploadDeniesOtherServicePeers(t *testing.T) {
	for _, identity := range []string{mtls.Auth, mtls.Core, mtls.ML, "spiffe://saldo.local/untrusted"} {
		t.Run(identity, func(t *testing.T) {
			called := make(chan struct{}, 1)
			address, ca := startTransportServer(t, &uploadProbe{upload: func(grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
				called <- struct{}{}
				return status.Error(codes.Internal, "unauthorized endpoint invocation")
			}})
			conn := transportClient(t, address, ca, identity)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			stream, err := pb.NewImportsServiceClient(conn).UploadImport(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = stream.CloseAndRecv()
			if status.Code(err) != codes.PermissionDenied {
				t.Fatal("non-API peer was not denied", err)
			}
			select {
			case <-called:
				t.Fatal("unauthorized stream reached endpoint")
			default:
			}
		})
	}
}

func TestUploadRejectsFrameAboveTwoMiB(t *testing.T) {
	address, ca := startTransportServer(t, &uploadProbe{upload: func(stream grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
		_, err := stream.Recv()
		return err
	}})
	conn := transportClient(t, address, ca, mtls.API)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	stream, err := pb.NewImportsServiceClient(conn).UploadImport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Client accepts this wire size, so the receiving server must reject it.
	_ = stream.Send(&pb.UploadImportRequest{Frame: &pb.UploadImportRequest_Chunk{Chunk: make([]byte, 2<<20)}})
	_, err = stream.CloseAndRecv()
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal("frame above server transport bound was not rejected", err)
	}
}

func TestUploadRejectsForeignCA(t *testing.T) {
	address, _ := startTransportServer(t, &uploadProbe{upload: func(grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
		return status.Error(codes.Internal, "foreign CA reached endpoint")
	}})
	conn := transportClient(t, address, pki.New(t), mtls.API)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	stream, err := pb.NewImportsServiceClient(conn).UploadImport(ctx)
	if err == nil {
		_, err = stream.CloseAndRecv()
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatal("foreign CA was accepted", err)
	}
}

func TestUploadPropagatesDeadlineAndCancellation(t *testing.T) {
	for _, mode := range []string{"deadline", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan time.Time, 1)
			finished := make(chan struct{}, 1)
			address, ca := startTransportServer(t, &uploadProbe{upload: func(stream grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
				deadline, _ := stream.Context().Deadline()
				started <- deadline
				<-stream.Context().Done()
				finished <- struct{}{}
				return status.FromContextError(stream.Context().Err()).Err()
			}})
			conn := transportClient(t, address, ca, mtls.API)
			duration, want := 3*time.Second, codes.Canceled
			if mode == "deadline" {
				duration, want = 500*time.Millisecond, codes.DeadlineExceeded
			}
			ctx, cancel := context.WithTimeout(t.Context(), duration)
			defer cancel()
			stream, err := pb.NewImportsServiceClient(conn).UploadImport(ctx)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case deadline := <-started:
				if callerDeadline, _ := ctx.Deadline(); deadline.After(callerDeadline.Add(50 * time.Millisecond)) {
					t.Fatal("server extended caller deadline", deadline, callerDeadline)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("upload endpoint did not start")
			}
			if mode == "cancellation" {
				cancel()
			}
			_, err = stream.CloseAndRecv()
			if status.Code(err) != want {
				t.Fatal("wrong interrupted upload status", err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("server stream survived caller interruption")
			}
		})
	}
}

type stalledTransportStream struct {
	grpc.ServerStream
	transportContext context.Context
	finished         chan struct{}
}

func (s stalledTransportStream) RecvMsg(any) error {
	defer close(s.finished)
	<-s.transportContext.Done()
	return s.transportContext.Err()
}

func TestBoundedReceiveReturnsBeforeTransportCancellation(t *testing.T) {
	transportContext, stopTransport := context.WithCancel(t.Context())
	defer stopTransport()
	finished := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	stream := boundedStream{ServerStream: stalledTransportStream{transportContext: transportContext, finished: finished}, ctx: ctx}
	result := make(chan error, 1)
	go func() { result <- stream.RecvMsg(&pb.UploadImportRequest{}) }()
	select {
	case err := <-result:
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatal("bounded receive did not stop at its own deadline", err)
		}
	case <-time.After(time.Second):
		stopTransport()
		<-result
		t.Fatal("receive ignored its deadline until transport cancellation")
	}
	if transportContext.Err() != nil {
		t.Fatal("test transport ended before handler deadline")
	}
	// gRPC cancels the original stream when the handler returns. Reproduce that
	// event and verify the in-flight receive can finish without blocking its send.
	stopTransport()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("in-flight transport receive did not finish")
	}
}

func startTransportServer(t *testing.T, imports pb.ImportsServiceServer) (string, *pki.Authority) {
	t.Helper()
	ca := pki.New(t)
	config, err := mtls.Server(ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(credentials.NewTLS(config))
	if err != nil {
		t.Fatal(err)
	}
	pb.RegisterImportsServiceServer(server, imports)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Error("test gRPC server did not stop")
		}
	})
	return listener.Addr().String(), ca
}

func transportClient(t *testing.T, address string, ca *pki.Authority, identity string) *grpc.ClientConn {
	t.Helper()
	config, err := mtls.Client(ca.Files(t, identity), "saldo-core", identity, mtls.Core)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(3<<20)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
