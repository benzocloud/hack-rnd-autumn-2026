package securetls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
)

// Load keeps normal chain/hostname verification and additionally pins the peer URI.
func Load(caFile, certFile, keyFile, peerURI string, server bool) (*tls.Config, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("cannot read TLS CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid TLS CA")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.New("cannot load TLS identity")
	}
	if peerURI == "" {
		return nil, errors.New("TLS peer URI is required")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: roots}
	if server {
		cfg.ClientCAs = roots
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	cfg.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
			return errors.New("unverified TLS peer")
		}
		uris := state.PeerCertificates[0].URIs
		if len(uris) != 1 || uris[0].String() != peerURI {
			return errors.New("unexpected TLS peer identity")
		}
		return nil
	}
	return cfg, nil
}
