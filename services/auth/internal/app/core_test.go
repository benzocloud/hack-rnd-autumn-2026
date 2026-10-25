package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/client/core"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type tlsCoreResult struct {
	pb.UnimplementedCompanyServiceServer
	receipt *pb.CompanyCreationReceipt
}

func (s *tlsCoreResult) GetCompanyCreationResult(context.Context, *pb.GetCompanyCreationResultRequest) (*pb.GetCompanyCreationResultResponse, error) {
	return &pb.GetCompanyCreationResultResponse{Receipt: s.receipt}, nil
}

func TestAuthCoreClientRequiresVerifiedCoreMTLSIdentity(t *testing.T) {
	for _, tt := range []struct {
		name, serverURI, clientURI, serverName string
		want                                   bool
	}{
		{"verified Core", "spiffe://saldo.local/core", "spiffe://saldo.local/auth", "core.saldo.test", true},
		{"wrong peer URI", "spiffe://saldo.local/api", "spiffe://saldo.local/auth", "core.saldo.test", false},
		{"wrong hostname", "spiffe://saldo.local/core", "spiffe://saldo.local/auth", "wrong.saldo.test", false},
		{"wrong local identity", "spiffe://saldo.local/core", "spiffe://saldo.local/api", "core.saldo.test", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			folder := t.TempDir()
			caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
			der, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
			if err != nil {
				t.Fatal(err)
			}
			ca, err = x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			caFile := filepath.Join(folder, "ca.pem")
			if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
				t.Fatal(err)
			}
			roots := x509.NewCertPool()
			roots.AddCert(ca)
			serverPair, _, _ := coreTestIdentity(t, folder, "server", 2, tt.serverURI, ca, caKey)
			_, clientCert, clientKey := coreTestIdentity(t, folder, "client", 3, tt.clientURI, ca, caKey)
			lc := net.ListenConfig{}
			listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			rpc := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverPair}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert})))
			spec := account.CompanySpec{Name: "Company", Currency: "RUB", Timezone: "Europe/Moscow"}
			hash, err := spec.Fingerprint()
			if err != nil {
				t.Fatal(err)
			}
			command := uuid.New()
			receipt := &pb.CompanyCreationReceipt{CommandId: command.String(), CompanyId: uuid.Must(uuid.NewV7()).String(), DefaultAccountId: uuid.Must(uuid.NewV7()).String(), CompanySpec: &authpb.CompanyCreateSpec{Name: spec.Name, Currency: spec.Currency, Timezone: spec.Timezone}, RequestSha256: hash, CommittedAt: timestamppb.Now()}
			pb.RegisterCompanyServiceServer(rpc, &tlsCoreResult{receipt: receipt})
			done := make(chan error, 1)
			go func() { done <- rpc.Serve(listener) }()
			t.Cleanup(func() {
				rpc.Stop()
				_ = listener.Close()
				if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
					t.Error(err)
				}
			})
			cfg := config.Config{CoreAddress: listener.Addr().String(), CoreServerName: tt.serverName, GRPCCertFile: clientCert, GRPCKeyFile: clientKey, GRPCCAFile: caFile, CoreTimeout: 300 * time.Millisecond}
			conn, err := openCore(cfg)
			if tt.clientURI != "spiffe://saldo.local/auth" {
				if err == nil {
					_ = conn.Close()
					t.Fatal("foreign local identity accepted")
				}
				return
			}
			if err != nil {
				t.Fatal("client initialization failed before peer handshake", err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			client, err := coreclient.New(conn, cfg.CoreTimeout)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetCreationResult(t.Context(), command)
			if (err == nil) != tt.want {
				t.Fatal(got, err)
			}
			if tt.want && (got.CommandID != command || got.CompanyID.String() != receipt.CompanyId) {
				t.Fatal(got)
			}
		})
	}
}

func coreTestIdentity(t *testing.T, folder, name string, serial int64, uri string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"core.saldo.test"}, URIs: []*url.URL{identity}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certFile, keyFile := filepath.Join(folder, name+".pem"), filepath.Join(folder, name+".key")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair, certFile, keyFile
}
