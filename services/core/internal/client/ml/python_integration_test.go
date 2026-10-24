//go:build mlintegration

package ml

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Requires the ML PR source and its pinned isolated Python environment. It never
// installs dependencies or downloads/executes a remote branch implicitly.
func TestPythonPredictMTLS(t *testing.T) {
	python, source := os.Getenv("CORE_ML_TEST_PYTHON"), os.Getenv("CORE_ML_TEST_SOURCE")
	if python == "" || source == "" {
		t.Skip("set CORE_ML_TEST_PYTHON and CORE_ML_TEST_SOURCE; see README")
	}
	config, certs := testTLS(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	script := `
import pathlib, sys
from cashflow_rpc.server import create_server
read = lambda name: pathlib.Path(sys.argv[1], name).read_bytes()
server, port = create_server(port=0, certificate=read("server.pem"), private_key=read("server.key"), client_ca=read("ca.pem"))
server.start()
print(port, flush=True)
server.wait_for_termination()
`
	command := exec.CommandContext(ctx, python, "-u", "-c", script, certs)
	command.Env = []string{"PATH=" + filepath.Dir(python), "PYTHONPATH=" + source, "PYTHONDONTWRITEBYTECODE=1"}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatalf("ML did not start: %v", scanner.Err())
	}
	port, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("invalid server port %q", scanner.Text())
	}
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c, err := New(conn, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Predict(ctx, inputFixture())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Days) != 30 || result.Days[0].ReferenceBalance.Minor() != 200 || result.Days[29].ReferenceBalance.Minor() != -2700 || result.FirstNegativeDate == nil || *result.FirstNegativeDate != "2026-10-27" {
		t.Fatalf("unexpected real ML result %+v", result)
	}
	input := inputFixture()
	input.History[0].ClosingBalance = rub(input.History[0].ClosingBalance.Minor() + 1)
	if _, err = c.Predict(ctx, input); err != nil {
		t.Fatalf("real reconciliation response: %v", err)
	}
}

func testTLS(t *testing.T) (*tls.Config, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated integration CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "ca.pem"), caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	var clientCert tls.Certificate
	for i, name := range []string{"server", "client"} {
		leafKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		dns := "saldo-ml"
		usage := x509.ExtKeyUsageServerAuth
		if name == "client" {
			dns = "saldo-core"
			usage = x509.ExtKeyUsageClientAuth
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), DNSNames: []string{dns}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		encoded, createErr := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if createErr != nil {
			t.Fatal(createErr)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded})
		keyDER, marshalErr := x509.MarshalPKCS8PrivateKey(leafKey)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		if err = os.WriteFile(filepath.Join(dir, name+".pem"), certPEM, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name+".key"), keyPEM, 0600); err != nil {
			t.Fatal(err)
		}
		if name == "client" {
			clientCert, err = tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "saldo-ml", Certificates: []tls.Certificate{clientCert}}, dir
}
