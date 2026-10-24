package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

const (
	API  = "spiffe://saldo.local/api"
	Core = "spiffe://saldo.local/core"
	Auth = "spiffe://saldo.local/auth"
	ML   = "spiffe://saldo.local/ml"
)

var ErrConfiguration = errors.New("invalid Core mTLS configuration")

type Files struct{ CA, Certificate, Key string }

func Server(files Files) (*tls.Config, error) {
	certificate, roots, err := load(files, Core)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}

func Client(files Files, serverName, ownIdentity, serverIdentity string) (*tls.Config, error) {
	if serverName == "" || (serverIdentity != Core && serverIdentity != Auth && serverIdentity != ML) {
		return nil, ErrConfiguration
	}
	certificate, roots, err := load(files, ownIdentity)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: serverName,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 || identity(state.VerifiedChains[0][0]) != serverIdentity {
				return ErrConfiguration
			}
			return nil
		}}, nil
}

// Identity only accepts the leaf of a TLS-verified chain. CN/metadata/request
// fields cannot choose a service identity; ambiguous URI SANs are rejected.
func Identity(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 || len(info.State.VerifiedChains[0]) == 0 {
		return ""
	}
	return identity(info.State.VerifiedChains[0][0])
}

func identity(certificate *x509.Certificate) string {
	if certificate == nil || len(certificate.URIs) != 1 {
		return ""
	}
	return certificate.URIs[0].String()
}

func load(files Files, ownIdentity string) (tls.Certificate, *x509.CertPool, error) {
	certificate, err := tls.LoadX509KeyPair(files.Certificate, files.Key)
	if err != nil || len(certificate.Certificate) == 0 {
		return tls.Certificate{}, nil, ErrConfiguration
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || identity(leaf) != ownIdentity {
		return tls.Certificate{}, nil, ErrConfiguration
	}
	data, err := os.ReadFile(files.CA)
	if err != nil {
		return tls.Certificate{}, nil, ErrConfiguration
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return tls.Certificate{}, nil, ErrConfiguration
	}
	return certificate, roots, nil
}
