package securetls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

func TestLoadFailures(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(bad, []byte("not PEM"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ ca, cert, key, peer string }{{"missing", p.APICert, p.APIKey, "peer"}, {bad, p.APICert, p.APIKey, "peer"}, {p.CA, "missing", p.APIKey, "peer"}, {p.CA, p.APICert, p.APIKey, ""}} {
		if _, err := Load(tc.ca, tc.cert, tc.key, tc.peer, false); err == nil {
			t.Fatal("bad TLS accepted")
		}
	}
	cfg, err := Load(p.CA, p.APICert, p.APIKey, "spiffe://saldo.local/auth", false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VerifyConnection(tls.ConnectionState{}) == nil {
		t.Fatal("unverified peer accepted")
	}
	if cfg.VerifyConnection(tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{}}, PeerCertificates: []*x509.Certificate{{}}}) == nil {
		t.Fatal("missing URI accepted")
	}
}

func TestMTLSTransportTrustAndIdentities(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, ca, cert, key, clientPeer, serverPeer string
		good                                        bool
	}{
		{"valid", p.CA, p.APICert, p.APIKey, "spiffe://saldo.local/auth", "spiffe://saldo.local/api", true},
		{"untrusted CA", other.CA, p.APICert, p.APIKey, "spiffe://saldo.local/auth", "spiffe://saldo.local/api", false},
		{"untrusted client", p.CA, other.APICert, other.APIKey, "spiffe://saldo.local/auth", "spiffe://saldo.local/api", false},
		{"wrong server URI", p.CA, p.APICert, p.APIKey, "spiffe://saldo.local/core", "spiffe://saldo.local/api", false},
		{"wrong client URI", p.CA, p.APICert, p.APIKey, "spiffe://saldo.local/auth", "spiffe://saldo.local/core", false},
		{"wrong EKU", p.CA, p.AuthCert, p.AuthKey, "spiffe://saldo.local/auth", "spiffe://saldo.local/api", false},
		{"missing client cert", p.CA, "", "", "spiffe://saldo.local/auth", "spiffe://saldo.local/api", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, err := Load(p.CA, p.AuthCert, p.AuthKey, tc.serverPeer, true)
			if err != nil {
				t.Fatal(err)
			}
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer lis.Close()
			srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
			defer srv.Stop()
			authv1.RegisterAuthServiceServer(srv, testauth.New())
			go func() { _ = srv.Serve(lis) }()
			var clientTLS *tls.Config
			if tc.cert == "" {
				pem, _ := os.ReadFile(p.CA)
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM(pem)
				clientTLS = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
			} else {
				clientTLS, err = Load(tc.ca, tc.cert, tc.key, tc.clientPeer, false)
				if err != nil {
					t.Fatal(err)
				}
			}
			conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)), grpc.WithDisableRetry())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			response, err := authv1.NewAuthServiceClient(conn).GetPersonalDataConsent(ctx, &authv1.GetPersonalDataConsentRequest{})
			if tc.good && (err != nil || response.GetDocument().GetText() != testauth.ConsentText) {
				t.Fatal("valid mTLS failed", err)
			}
			if !tc.good && err == nil {
				t.Fatal("untrusted peer accepted")
			}
		})
	}
}
