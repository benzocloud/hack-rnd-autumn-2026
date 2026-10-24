package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TLSCredentials requires the caller to prove a CA-issued service identity.
func TLSCredentials(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.New("auth TLS certificate unavailable")
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("auth TLS CA unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("auth TLS CA invalid")
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}), nil
}

// AuthenticatePeer runs before any business state is read. Request fields cannot select a service identity.
func AuthenticatePeer(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "service identity required")
	}
	identity, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(identity.State.VerifiedChains) == 0 || len(identity.State.PeerCertificates) == 0 {
		return nil, status.Error(codes.PermissionDenied, "service identity required")
	}
	uris := identity.State.PeerCertificates[0].URIs
	if len(uris) != 1 {
		return nil, status.Error(codes.PermissionDenied, "service identity required")
	}
	caller := uris[0].String()
	method := strings.TrimPrefix(info.FullMethod, "/auth.v1.AuthService/")
	allowed := false
	switch caller {
	case "spiffe://saldo.local/api":
		allowed = method != "AuthorizeCompanyBootstrap"
	case "spiffe://saldo.local/core":
		allowed = method == "AuthorizeCompany" || method == "AuthorizeCompanyBootstrap"
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "service permission denied")
	}
	timeout := 3 * time.Second
	if method == "Register" || method == "Login" || method == "ResetPassword" {
		timeout = 10 * time.Second
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return handler(bounded, request)
}
