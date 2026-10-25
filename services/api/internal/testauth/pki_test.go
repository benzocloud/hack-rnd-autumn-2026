package testauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("missing PEM certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}

func TestEphemeralPKIIdentityChainAndPermissions(t *testing.T) {
	paths, err := GeneratePKI(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	ca := readCertificate(t, paths.CA)
	if !ca.IsCA || ca.NotAfter.Sub(ca.NotBefore) > 25*time.Hour {
		t.Fatal("expected short-lived test CA")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, item := range []struct {
		name, certificate, key, uri string
		usage                       x509.ExtKeyUsage
	}{
		{"API", paths.APICert, paths.APIKey, "spiffe://saldo.local/api", x509.ExtKeyUsageClientAuth},
		{"Auth", paths.AuthCert, paths.AuthKey, "spiffe://saldo.local/auth", x509.ExtKeyUsageServerAuth},
		{"Core", paths.CoreCert, paths.CoreKey, "spiffe://saldo.local/core", x509.ExtKeyUsageServerAuth},
		{"HTTPS", paths.HTTPCert, paths.HTTPKey, "", x509.ExtKeyUsageServerAuth},
	} {
		t.Run(item.name, func(t *testing.T) {
			certificate := readCertificate(t, item.certificate)
			if _, loadErr := tls.LoadX509KeyPair(item.certificate, item.key); loadErr != nil {
				t.Fatalf("certificate/key mismatch: %v", loadErr)
			}
			if _, verifyErr := certificate.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{item.usage}}); verifyErr != nil {
				t.Fatalf("wrong trust chain or key purpose: %v", verifyErr)
			}
			if item.uri != "" && (len(certificate.URIs) != 1 || certificate.URIs[0].String() != item.uri) {
				t.Fatal("incorrect service URI identity")
			}
			if item.name == "Auth" || item.name == "Core" {
				if _, verifyErr := certificate.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); verifyErr != nil {
					t.Fatal("service identity cannot make authenticated outgoing calls")
				}
			}
			if item.usage == x509.ExtKeyUsageServerAuth {
				for _, hostname := range []string{"localhost", "127.0.0.1"} {
					if hostnameErr := certificate.VerifyHostname(hostname); hostnameErr != nil {
						t.Fatalf("loopback hostname missing: %v", hostnameErr)
					}
				}
				if certificate.VerifyHostname("production.example.test") == nil {
					t.Fatal("test certificate trusted an unrelated host")
				}
			}
			info, statErr := os.Stat(item.key)
			if statErr != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("private key permissions must be0600: %v", statErr)
			}
		})
	}
	second, err := GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ca.Equal(readCertificate(t, second.CA)) {
		t.Fatal("PKI generation reused a static CA")
	}
}

func TestPKIFilesystemFailures(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "regular-file")
	if err := os.WriteFile(file, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GeneratePKI(file); err == nil {
		t.Fatal("accepted a regular file as PKI directory")
	}
	for _, blocked := range []string{"ca.pem", "api.pem", "api-key.pem"} {
		t.Run(blocked, func(t *testing.T) {
			target := t.TempDir()
			if err := os.Mkdir(filepath.Join(target, blocked), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := GeneratePKI(target); err == nil {
				t.Fatalf("ignored unwritable credential destination %s", blocked)
			}
		})
	}
}

func TestCertificateRejectsMalformedIdentityAndInvalidSigner(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign,
	}
	directory := t.TempDir()
	cert, private := filepath.Join(directory, "leaf.pem"), filepath.Join(directory, "key.pem")
	if err := issueCertificate(ca, key, 2, "malformed", "spiffe://%", x509.ExtKeyUsageClientAuth, cert, private); err == nil {
		t.Fatal("accepted malformed URI SAN")
	}
	ca.PublicKey = &key.PublicKey
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := issueCertificate(ca, other, 3, "wrong signer", "", x509.ExtKeyUsageClientAuth, cert, private); err == nil {
		t.Fatal("signed with a key that does not match CA")
	}
}
