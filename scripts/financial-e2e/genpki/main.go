// Ephemeral loopback PKI for financial-e2e only. Not for production.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("out", "", "output directory (mode 0700)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: genpki -out <dir>")
		os.Exit(2)
	}
	if err := generate(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Saldo financial-e2e CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	pub, err := x509.MarshalPKIXPublicKey(&caKey.PublicKey)
	if err != nil {
		return err
	}
	keyID := sha256.Sum256(pub)
	ca.SubjectKeyId = keyID[:]
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caPath := filepath.Join(directory, "ca.pem")
	if err := writePEM(caPath, "CERTIFICATE", caDER); err != nil {
		return err
	}
	leaves := []struct {
		file, key, name, uri, dns string
		client, server            bool
	}{
		{"api.pem", "api-key.pem", "e2e API", "spiffe://saldo.local/api", "", true, false},
		{"auth.pem", "auth-key.pem", "e2e Auth", "spiffe://saldo.local/auth", "localhost", true, true},
		{"core.pem", "core-key.pem", "e2e Core", "spiffe://saldo.local/core", "localhost", true, true},
		{"ml.pem", "ml-key.pem", "e2e ML", "spiffe://saldo.local/ml", "saldo-ml", true, true},
		{"https.pem", "https-key.pem", "e2e HTTPS", "", "localhost", false, true},
	}
	for i, leaf := range leaves {
		usage := []x509.ExtKeyUsage{}
		if leaf.client {
			usage = append(usage, x509.ExtKeyUsageClientAuth)
		}
		if leaf.server {
			usage = append(usage, x509.ExtKeyUsageServerAuth)
		}
		if err := issue(ca, caKey, int64(i+2), leaf.name, leaf.uri, leaf.dns, usage,
			filepath.Join(directory, leaf.file), filepath.Join(directory, leaf.key)); err != nil {
			return err
		}
	}
	fmt.Println(directory)
	return nil
}

func issue(ca *x509.Certificate, caKey *ecdsa.PrivateKey, serial int64, name, uri, dns string, usage []x509.ExtKeyUsage, certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             ca.NotBefore,
		NotAfter:              ca.NotAfter,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           usage,
	}
	for _, item := range usage {
		if item == x509.ExtKeyUsageServerAuth {
			cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
			if dns != "" {
				cert.DNSNames = []string{dns, "localhost"}
			} else {
				cert.DNSNames = []string{"localhost"}
			}
		}
	}
	if uri != "" {
		identity, err := url.Parse(uri)
		if err != nil {
			return err
		}
		cert.URIs = []*url.URL{identity}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", der); err != nil {
		return err
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(keyPath, "PRIVATE KEY", encoded)
}

func writePEM(path, kind string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600)
}
