//go:build onboardingintegration

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/core"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

// Only the transport boundary is controlled: successful calls still commit in
// the real Core process. Outgoing certificates preserve API/Auth peer identity.
type onboardingProxy struct {
	corev1.UnimplementedCompanyServiceServer
	corev1.UnimplementedLedgerServiceServer
	api, auth     corev1.CompanyServiceClient
	ledger        corev1.LedgerServiceClient
	holdBootstrap atomic.Bool
	holdReceipt   atomic.Bool
	dropReply     atomic.Bool
}

func startOnboardingProxy(t *testing.T, pki testauth.PKI, address, target string) *onboardingProxy {
	t.Helper()
	ca, err := os.ReadFile(pki.CA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("invalid test CA")
	}
	cert, err := tls.LoadX509KeyPair(pki.CoreCert, pki.CoreKey)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots})))
	api, err := coreclient.Dial(target, "localhost", pki.CA, pki.APICert, pki.APIKey, 2e9)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := coreclient.Dial(target, "localhost", pki.CA, pki.AuthCert, pki.AuthKey, 2e9)
	if err != nil {
		t.Fatal(err)
	}
	p := &onboardingProxy{api: corev1.NewCompanyServiceClient(api), auth: corev1.NewCompanyServiceClient(auth), ledger: corev1.NewLedgerServiceClient(api)}
	corev1.RegisterCompanyServiceServer(srv, p)
	corev1.RegisterLedgerServiceServer(srv, p)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { srv.Stop(); _ = listener.Close(); _ = api.Close(); _ = auth.Close() })
	return p
}

func onboardingPeer(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 || len(info.State.PeerCertificates) == 0 || len(info.State.PeerCertificates[0].URIs) != 1 {
		return ""
	}
	return info.State.PeerCertificates[0].URIs[0].String()
}

func (p *onboardingProxy) CreateCompanyBootstrap(ctx context.Context, r *corev1.CreateCompanyBootstrapRequest) (*corev1.CreateCompanyBootstrapResponse, error) {
	identity := onboardingPeer(ctx)
	if identity != "spiffe://saldo.local/api" && identity != "spiffe://saldo.local/auth" {
		return nil, status.Error(codes.PermissionDenied, "peer denied")
	}
	if p.holdBootstrap.Load() {
		return nil, status.Error(codes.Unavailable, "test bootstrap transport unavailable")
	}
	client := p.api
	if identity == "spiffe://saldo.local/auth" {
		client = p.auth
	}
	response, err := client.CreateCompanyBootstrap(ctx, r)
	if err == nil && identity == "spiffe://saldo.local/api" && p.dropReply.Swap(false) {
		return nil, status.Error(codes.Unavailable, "test reply lost after commit")
	}
	return response, err
}

func (p *onboardingProxy) GetCompanyCreationResult(ctx context.Context, r *corev1.GetCompanyCreationResultRequest) (*corev1.GetCompanyCreationResultResponse, error) {
	if onboardingPeer(ctx) != "spiffe://saldo.local/auth" {
		return nil, status.Error(codes.PermissionDenied, "peer denied")
	}
	if p.holdReceipt.Load() {
		return nil, status.Error(codes.Unavailable, "test receipt transport unavailable")
	}
	return p.auth.GetCompanyCreationResult(ctx, r)
}

func (p *onboardingProxy) ReadCompany(ctx context.Context, r *corev1.ReadCompanyRequest) (*corev1.ReadCompanyResponse, error) {
	if onboardingPeer(ctx) != "spiffe://saldo.local/api" {
		return nil, status.Error(codes.PermissionDenied, "peer denied")
	}
	return p.api.ReadCompany(ctx, r)
}

func (p *onboardingProxy) GetDefaultAccount(ctx context.Context, r *corev1.GetDefaultAccountRequest) (*corev1.GetDefaultAccountResponse, error) {
	if onboardingPeer(ctx) != "spiffe://saldo.local/api" {
		return nil, status.Error(codes.PermissionDenied, "peer denied")
	}
	return p.ledger.GetDefaultAccount(ctx, r)
}
