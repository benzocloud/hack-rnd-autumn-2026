package testauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type PKI struct {
	CA       string
	APICert  string
	APIKey   string
	AuthCert string
	AuthKey  string
	CoreCert string
	CoreKey  string
	HTTPCert string
	HTTPKey  string
}

// GeneratePKI writes ephemeral, synthetic credentials for loopback tests only.
func GeneratePKI(directory string) (PKI, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return PKI{}, fmt.Errorf("create test PKI directory: %w", err)
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return PKI{}, fmt.Errorf("generate test CA key: %w", err)
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Saldo ephemeral test CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	// Explicit SKID lets leaf certificates receive an AKID even before the CA
	// template is parsed back; required by strict OpenSSL verification.
	publicKey, err := x509.MarshalPKIXPublicKey(&caKey.PublicKey)
	if err != nil {
		return PKI{}, fmt.Errorf("marshal CA public key: %w", err)
	}
	keyID := sha256.Sum256(publicKey)
	ca.SubjectKeyId = keyID[:]
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return PKI{}, fmt.Errorf("sign test CA: %w", err)
	}
	paths := PKI{
		CA:      filepath.Join(directory, "ca.pem"),
		APICert: filepath.Join(directory, "api.pem"), APIKey: filepath.Join(directory, "api-key.pem"),
		AuthCert: filepath.Join(directory, "auth.pem"), AuthKey: filepath.Join(directory, "auth-key.pem"),
		CoreCert: filepath.Join(directory, "core.pem"), CoreKey: filepath.Join(directory, "core-key.pem"),
		HTTPCert: filepath.Join(directory, "https.pem"), HTTPKey: filepath.Join(directory, "https-key.pem"),
	}
	if err := writePEM(paths.CA, "CERTIFICATE", caDER); err != nil {
		return PKI{}, err
	}
	certificates := []struct {
		name, uri, certificate, key string
		usage                       x509.ExtKeyUsage
	}{
		{"test API", "spiffe://saldo.local/api", paths.APICert, paths.APIKey, x509.ExtKeyUsageClientAuth},
		{"test Auth", "spiffe://saldo.local/auth", paths.AuthCert, paths.AuthKey, x509.ExtKeyUsageServerAuth},
		{"test Core", "spiffe://saldo.local/core", paths.CoreCert, paths.CoreKey, x509.ExtKeyUsageServerAuth},
		{"test HTTPS", "", paths.HTTPCert, paths.HTTPKey, x509.ExtKeyUsageServerAuth},
	}
	for i, item := range certificates {
		if err := issueCertificate(ca, caKey, int64(i+2), item.name, item.uri, item.usage, item.certificate, item.key); err != nil {
			return PKI{}, err
		}
	}
	return paths, nil
}

func issueCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey, serial int64, name, uri string, usage x509.ExtKeyUsage, certificatePath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate test leaf key: %w", err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
		NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	if usage == x509.ExtKeyUsageServerAuth {
		certificate.DNSNames = []string{"localhost"}
		certificate.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	if uri != "" {
		identity, parseErr := url.Parse(uri)
		if parseErr != nil {
			return fmt.Errorf("parse test URI SAN: %w", parseErr)
		}
		certificate.URIs = []*url.URL{identity}
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, ca, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("sign test certificate: %w", err)
	}
	if err := writePEM(certificatePath, "CERTIFICATE", der); err != nil {
		return err
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal test private key: %w", err)
	}
	return writePEM(keyPath, "PRIVATE KEY", encodedKey)
}

func writePEM(path, kind string, der []byte) error {
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
		return fmt.Errorf("write test PKI: %w", err)
	}
	return nil
}
