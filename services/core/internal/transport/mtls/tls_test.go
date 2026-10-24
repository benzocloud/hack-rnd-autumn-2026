package mtls_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func TestTLSConfigurationFailsClosed(t *testing.T) {
	if _, err := mtls.Server(mtls.Files{}); !errors.Is(err, mtls.ErrConfiguration) {
		t.Fatal(err)
	}
	ca := pki.New(t)
	files := ca.Files(t, mtls.API)
	if _, err := mtls.Server(files); !errors.Is(err, mtls.ErrConfiguration) {
		t.Fatal("API certificate accepted by Core server", err)
	}
	if _, err := mtls.Client(files, "saldo-auth", mtls.Core, mtls.Auth); !errors.Is(err, mtls.ErrConfiguration) {
		t.Fatal("wrong local identity accepted", err)
	}
	if mtls.Identity(t.Context()) != "" {
		t.Fatal("context invented service identity")
	}
	if _, err := mtls.Client(ca.Files(t, mtls.Core), "saldo-ml", mtls.Core, mtls.ML); err != nil {
		t.Fatal("valid ML peer configuration rejected", err)
	}
	if _, err := mtls.Client(files, "saldo-api", mtls.API, mtls.API); !errors.Is(err, mtls.ErrConfiguration) {
		t.Fatal("unsupported server identity accepted", err)
	}
}

func TestTLSRejectsMissingClientAndWrongServerIdentity(t *testing.T) {
	for _, mode := range []string{"missing client certificate", "wrong server identity", "foreign CA"} {
		t.Run(mode, func(t *testing.T) {
			ca := pki.New(t)
			serverIdentity := mtls.Auth
			if mode == "wrong server identity" {
				serverIdentity = mtls.API
			}
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, ca.Files(t, serverIdentity)))))
			healthpb.RegisterHealthServer(server, health.NewServer())
			listener := bufconn.Listen(1 << 20)
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			clientCA := ca
			if mode == "foreign CA" {
				clientCA = pki.New(t)
			}
			clientTLS, err := mtls.Client(clientCA.Files(t, mtls.Core), "saldo-auth", mtls.Core, mtls.Auth)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing client certificate" {
				clientTLS.Certificates = nil
			}
			conn, err := grpc.NewClient("passthrough:///mtls-test", grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if _, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Unavailable {
				t.Fatal("invalid TLS peer accepted", err)
			}
		})
	}
}
