package app

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/client/core"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

// openCore uses the existing Auth service identity. Normal chain/hostname
// verification remains enabled; the additional URI pin is Core-only.
func openCore(cfg config.Config) (*grpc.ClientConn, error) {
	pair, err := tls.LoadX509KeyPair(cfg.GRPCCertFile, cfg.GRPCKeyFile)
	if err != nil {
		return nil, errors.New("auth Core client identity unavailable")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || len(leaf.URIs) != 1 || leaf.URIs[0].String() != "spiffe://saldo.local/auth" {
		return nil, errors.New("auth Core client identity invalid")
	}
	pem, err := os.ReadFile(cfg.GRPCCAFile)
	if err != nil {
		return nil, errors.New("auth Core client CA unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("auth Core client CA invalid")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}, ServerName: cfg.CoreServerName, VerifyConnection: verifyCorePeer}
	return grpc.NewClient(cfg.CoreAddress, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithDisableRetry())
}

func verifyCorePeer(state tls.ConnectionState) error {
	if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return errors.New("unverified Core peer")
	}
	uri := state.PeerCertificates[0].URIs
	if len(uri) != 1 || uri[0].String() != "spiffe://saldo.local/core" {
		return errors.New("unexpected Core peer identity")
	}
	return nil
}

func (app *application) configureCore(cfg config.Config, options account.Options) account.Options {
	if cfg.CoreAddress == "" {
		return options
	}
	connection, err := openCore(cfg)
	if err != nil {
		app.coreUnavailable = true
		return options
	}
	client, err := coreclient.New(connection, cfg.CoreTimeout)
	if err != nil {
		_ = connection.Close()
		app.coreUnavailable = true
		return options
	}
	app.coreConnection = connection
	options.Core = client
	return options
}
