package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type Authority struct {
	root   *x509.Certificate
	key    *ecdsa.PrivateKey
	ca     []byte
	dir    string
	serial int64
}

// New creates short-lived test certificates only inside t.TempDir.
func New(tb testing.TB) *Authority {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated test CA"}, NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		tb.Fatal(err)
	}
	return &Authority{root: root, key: key, ca: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), dir: tb.TempDir(), serial: 1}
}

func (a *Authority) Files(tb testing.TB, identity string) mtls.Files {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	uri, err := url.Parse(identity)
	if err != nil {
		tb.Fatal(err)
	}
	a.serial++
	leaf := &x509.Certificate{SerialNumber: big.NewInt(a.serial), URIs: []*url.URL{uri}, DNSNames: []string{"saldo-core", "saldo-auth"},
		NotBefore: a.root.NotBefore, NotAfter: a.root.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, a.root, &key.PublicKey, a.key)
	if err != nil {
		tb.Fatal(err)
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		tb.Fatal(err)
	}
	dir := filepath.Join(a.dir, big.NewInt(a.serial).String())
	if err := os.Mkdir(dir, 0700); err != nil {
		tb.Fatal(err)
	}
	files := mtls.Files{CA: filepath.Join(dir, "ca.pem"), Certificate: filepath.Join(dir, "cert.pem"), Key: filepath.Join(dir, "key.pem")}
	for path, data := range map[string][]byte{files.CA: a.ca, files.Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), files.Key: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey})} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			tb.Fatal(err)
		}
	}
	return files
}

func Server(tb testing.TB, files mtls.Files) *tls.Config {
	tb.Helper()
	certificate, err := tls.LoadX509KeyPair(files.Certificate, files.Key)
	if err != nil {
		tb.Fatal(err)
	}
	data, err := os.ReadFile(files.CA)
	if err != nil {
		tb.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		tb.Fatal("invalid test CA")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}
}
