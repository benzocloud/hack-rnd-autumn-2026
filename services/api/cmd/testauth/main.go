package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

func main() {
	address := flag.String("address", "127.0.0.1:8082", "loopback gRPC address")
	dir := flag.String("pki", "", "ephemeral certificate directory (required)")
	flag.Parse()
	if err := run(*address, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "test Auth failed:", err)
		os.Exit(1)
	}
}
func run(address, dir string) error {
	if dir == "" {
		return fmt.Errorf("pki directory required")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("test Auth requires loopback")
	}
	pki, err := testauth.GeneratePKI(dir)
	if err != nil {
		return err
	}
	tlsConfig, err := securetls.Load(pki.CA, pki.AuthCert, pki.AuthKey, "spiffe://saldo.local/api", true)
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", address)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(2<<20))
	authv1.RegisterAuthServiceServer(srv, testauth.New())
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() { <-ctx.Done(); srv.Stop() }()
	fmt.Println("TEST FIXTURE ONLY: no persistence, real accounts or email delivery; listening", listener.Addr())
	return srv.Serve(listener)
}
