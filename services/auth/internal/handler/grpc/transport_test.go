package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type modeAccounts struct {
	accountOperations
	calls atomic.Int32
}

func (a *modeAccounts) GetSecurityContext(context.Context, string) (account.SecurityContext, error) {
	a.calls.Add(1)
	return account.SecurityContext{CookieToken: "private-cookie", CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (*modeAccounts) EmailVerificationMode() string { return "demo" }

func TestMTLSPeerCannotSelectServiceIdentity(t *testing.T) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Auth test CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	folder := t.TempDir()
	caPath := filepath.Join(folder, "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	serverCert, certFile, keyFile := testCertificate(t, folder, "server", 2, "spiffe://saldo.local/auth", ca, caKey)
	_ = serverCert
	creds, err := TLSCredentials(certFile, keyFile, caPath)
	if err != nil {
		t.Fatal(err)
	}
	lc := net.ListenConfig{}
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accounts := &modeAccounts{}
	srv := grpcgo.NewServer(grpcgo.Creds(creds), grpcgo.UnaryInterceptor(AuthenticatePeer))
	pb.RegisterAuthServiceServer(srv, New(accounts))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = listener.Close()
		if err := <-served; err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
			t.Error("test RPC server failed")
		}
	})
	for index, tc := range []struct {
		name, uri string
		want      codes.Code
	}{
		{"API identity", "spiffe://saldo.local/api", codes.OK},
		{"Core cannot register browser context", "spiffe://saldo.local/core", codes.PermissionDenied},
		{"Other service cannot impersonate API", "spiffe://saldo.local/frontend", codes.PermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert, _, _ := testCertificate(t, folder, tc.name, int64(index+3), tc.uri, ca, caKey)
			client, err := grpcgo.NewClient(listener.Addr().String(), grpcgo.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, ServerName: "auth.saldo.test", RootCAs: roots, Certificates: []tls.Certificate{cert}})))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = client.Close() }()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			reply, err := pb.NewAuthServiceClient(client).GetSecurityContext(ctx, &pb.GetSecurityContextRequest{Client: &pb.ClientContext{ClientIp: "spiffe://saldo.local/api"}})
			if status.Code(err) != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want == codes.OK && (reply.GetEmailVerificationMode() != "demo" || reply.GetContext().GetCookieToken() != "private-cookie") {
				t.Fatal("server-owned mode/context not returned")
			}
		})
	}
	if accounts.calls.Load() != 1 {
		t.Fatal("denied peers reached business state")
	}
}

func testCertificate(t *testing.T, folder, name string, serial int64, uri string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"auth.saldo.test"}, URIs: []*url.URL{parsed}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certPath, keyPath := filepath.Join(folder, name+".pem"), filepath.Join(folder, name+".key")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair, certPath, keyPath
}

func TestRPCErrorPreservesReasonWithoutSecrets(t *testing.T) {
	err := rpcError(errors.Join(account.ErrCSRFInvalid, errors.New("TOP_SECRET")))
	s := status.Convert(err)
	if s.Code() != codes.PermissionDenied || strings.Contains(err.Error(), "TOP_SECRET") {
		t.Fatal("unsafe status mapping")
	}
	if len(s.Details()) != 1 {
		t.Fatal("missing structured reason")
	}
	detail, ok := s.Details()[0].(*pb.AuthErrorDetail)
	if !ok || detail.GetReason() != pb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID {
		t.Fatal("wrong structured reason")
	}
}
