//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	authClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/auth"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func TestAccountRPCShowsOnlyAcceptedDatedBalance(t *testing.T) {
	application, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, application, cmd.CompanyID)
	companies := newCompanies(application.db, cfg.Postgres, authorizeCommand(cmd))
	created, err := companies.Bootstrap(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.Scope{CompanyID: created.CompanyID, AccountID: created.AccountID}
	ca := pki.New(t)
	authServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, ca.Files(t, mtls.Auth)))))
	authpb.RegisterAuthServiceServer(authServer, &bootstrapAuthServer{command: cmd})
	auth, err := authClient.New(testRPCConnection(t, ca, authServer, mtls.Core, mtls.Auth, "saldo-auth"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	serverTLS, err := mtls.Server(ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(serverTLS))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerHandler.Register(server, application.Ledger, companies, auth); err != nil {
		t.Fatal(err)
	}
	client := pb.NewLedgerServiceClient(testRPCConnection(t, ca, server, mtls.API, mtls.Core, "saldo-core"))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request := &pb.GetDefaultAccountRequest{CompanyId: cmd.CompanyID.String(), Context: &authpb.BrowserContext{CookieToken: "live-session"}}
	response, err := client.GetDefaultAccount(ctx, request)
	if err != nil || response.Account.Id != created.AccountID.String() || response.Account.Balance != nil || response.Account.TransactionSource != nil || response.Account.DataVersion != 1 {
		t.Fatal(response, err)
	}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	today := domain.Date(now.Format(time.DateOnly))
	writeBalance := func(date domain.Date, amount, version int64, source domain.BalanceSource) {
		t.Helper()
		_, err := application.Ledger.RecordBalance(ctx, ledger.BalanceCommand{Scope: scope, Date: date, Amount: rub(t, amount), Source: source, Note: "test EOD",
			ExpectedDataVersion: version, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
	}
	writeBalance(today, -500, 1, domain.BalanceSource{Kind: domain.ManualEOD})
	response, err = client.GetDefaultAccount(ctx, request)
	if err != nil || response.Account.Balance.GetAmount().GetAmountMinor() != -500 || response.Account.Balance.AsOf != string(today) || response.Account.Balance.Source != pb.BalanceSource_BALANCE_SOURCE_MANUAL || response.Account.TransactionSource != nil || response.Account.DataVersion != 2 {
		t.Fatal(response, err)
	}
	fact := transactionCommand(t, scope)
	fact.Record.Scope = scope
	fact.Record.Date = today
	fact.ExpectedDataVersion = 2
	if _, err := application.Ledger.CreateTransaction(ctx, fact); err != nil {
		t.Fatal(err)
	}
	writeBalance(domain.Date(now.AddDate(0, 0, -2).Format(time.DateOnly)), 2000, 3, domain.BalanceSource{Kind: domain.ManualEOD})
	writeBalance(domain.Date(now.AddDate(0, 0, 1).Format(time.DateOnly)), 9000, 4, domain.BalanceSource{Kind: domain.ManualEOD})
	response, err = client.GetDefaultAccount(ctx, request)
	if err != nil || response.Account.Balance.Amount.AmountMinor != -500 || response.Account.Balance.AsOf != string(today) || response.Account.DataVersion != 5 || response.Account.TransactionSource.GetMode() != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL || response.Account.TransactionSource.SourceSystem != "manual_v1" || response.Account.TransactionSource.SourceCompanyId != nil {
		t.Fatal("wrong latest EOD or fabricated balance/source", response, err)
	}
	// Read-only RPC must not advance the version or write a receipt.
	response, err = client.GetDefaultAccount(ctx, request)
	if err != nil || response.Account.DataVersion != 5 {
		t.Fatal(response, err)
	}
	request.Context.CookieToken = "revoked-session"
	if _, err := client.GetDefaultAccount(ctx, request); status.Code(err) != codes.PermissionDenied {
		t.Fatal("revoked user read finance", err)
	}
	request.Context.CookieToken = "live-session"
	request.CompanyId = uuid.NewString()
	if _, err := client.GetDefaultAccount(ctx, request); status.Code(err) != codes.NotFound {
		t.Fatal("foreign company was visible", err)
	}
	peerClient := pb.NewLedgerServiceClient(testRPCConnection(t, ca, server, mtls.Auth, mtls.Core, "saldo-core"))
	if _, err := peerClient.GetDefaultAccount(ctx, request); status.Code(err) != codes.PermissionDenied {
		t.Fatal("Auth service read account", err)
	}
}

func TestAccountReadStatementZeroAndTenantScope(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	_, err := application.Ledger.RecordBalance(t.Context(), ledger.BalanceCommand{Scope: scope, Date: "2026-10-23", Amount: rub(t, 0),
		Source: domain.BalanceSource{Kind: domain.StatementEOD, Namespace: domain.Namespace{System: "dataset_v1", CompanyID: "C001"}}, ExpectedDataVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	read, err := application.Ledger.ReadAccount(t.Context(), scope, "2026-10-24")
	if err != nil || read.Balance == nil || read.Balance.Amount().Minor() != 0 || read.Balance.Source().Kind != domain.StatementEOD || read.Account.Factual != domain.Imported || read.Account.Namespace.CompanyID != "C001" || read.Account.DataVersion != 2 {
		t.Fatal(read, err)
	}
	foreign := scope
	foreign.CompanyID = uuid.New()
	if _, err := application.Ledger.ReadAccount(t.Context(), foreign, "2026-10-24"); err == nil {
		t.Fatal("cross-tenant account resolved")
	}
}
