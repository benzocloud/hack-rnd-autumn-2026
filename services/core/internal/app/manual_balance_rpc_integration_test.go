//go:build integration

package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

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

type manualBalanceFixture struct {
	app            *application
	scope          domain.Scope
	today          domain.Date
	request        *pb.RecordManualBalanceRequest
	ca             *pki.Authority
	authConnection *grpc.ClientConn
}

func newManualBalanceFixture(t *testing.T) *manualBalanceFixture {
	t.Helper()
	application, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, application, cmd.CompanyID)
	created, err := newCompanies(application.db, cfg.Postgres, authorizeCommand(cmd)).Bootstrap(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	ca := pki.New(t)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, ca.Files(t, mtls.Auth)))))
	authpb.RegisterAuthServiceServer(server, &bootstrapAuthServer{command: cmd})
	connection := testRPCConnection(t, ca, server, mtls.Core, mtls.Auth, "saldo-auth")
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	scope := domain.Scope{CompanyID: created.CompanyID, AccountID: created.AccountID}
	request := &pb.RecordManualBalanceRequest{Scope: &pb.AccountScope{CompanyId: scope.CompanyID.String(), AccountId: scope.AccountID.String()}, IdempotencyKey: uuid.NewString(), ExpectedDataVersion: 1,
		Balance: &pb.Money{AmountMinor: -500, Currency: "RUB"}, AsOf: now.AddDate(0, 0, -3).Format(time.DateOnly), Note: "Начальный EOD",
		Context: &authpb.WriteContext{Browser: &authpb.BrowserContext{CookieToken: "live-session"}, CsrfToken: "valid-csrf"}}
	return &manualBalanceFixture{app: application, scope: scope, today: domain.Date(now.Format(time.DateOnly)), request: request, ca: ca, authConnection: connection}
}

func (f *manualBalanceFixture) client(t *testing.T, application *application, identity string) pb.LedgerServiceClient {
	t.Helper()
	auth, err := authClient.New(f.authConnection, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := mtls.Server(f.ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(tlsConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerHandler.Register(server, application.Ledger, application.Companies, auth); err != nil {
		t.Fatal(err)
	}
	return pb.NewLedgerServiceClient(testRPCConnection(t, f.ca, server, identity, mtls.Core, "saldo-core"))
}

func TestManualBalanceRPCConcurrentReplaySurvivesMutableAccount(t *testing.T) {
	f := newManualBalanceFixture(t)
	client := f.client(t, f.app, mtls.API)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	results := make([]*pb.RecordManualBalanceResponse, 8)
	errs := make([]error, len(results))
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i], errs[i] = client.RecordManualBalance(ctx, f.request) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || !proto.Equal(results[0], results[i]) {
			t.Fatal("concurrent retry changed response", i, err)
		}
	}
	first := results[0]
	if first.Account.DataVersion != 2 || first.Account.Balance.GetAmount().GetAmountMinor() != -500 || first.Account.Balance.AsOf != f.request.AsOf || first.Account.TransactionSource != nil {
		t.Fatal(first)
	}
	var receipts, balances int
	if err := f.app.db.GetContext(ctx, &receipts, "SELECT count(*) FROM core_command_results WHERE company_id=$1 AND operation='record_manual_balance_v1'", f.scope.CompanyID); err != nil || receipts != 1 {
		t.Fatal(receipts, err)
	}
	if err := f.app.db.GetContext(ctx, &balances, "SELECT count(*) FROM core_balances WHERE company_id=$1", f.scope.CompanyID); err != nil || balances != 1 {
		t.Fatal(balances, err)
	}
	fact := transactionCommand(t, f.scope)
	fact.Record.Date = f.today
	fact.ExpectedDataVersion = 2
	if _, err := f.app.Ledger.CreateTransaction(ctx, fact); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Ledger.RecordBalance(ctx, ledger.BalanceCommand{Scope: f.scope, Date: f.today, Amount: rub(t, 9000), Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "later EOD", ExpectedDataVersion: 3, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	current, err := client.GetDefaultAccount(ctx, &pb.GetDefaultAccountRequest{CompanyId: f.scope.CompanyID.String(), Context: f.request.Context.Browser})
	if err != nil || current.Account.DataVersion != 4 || current.Account.TransactionSource.GetMode() != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL || current.Account.Balance.Amount.AmountMinor != 9000 {
		t.Fatal(current, err)
	}
	second, _ := testApplication(t)
	restarted := f.client(t, second, mtls.API)
	replayed, err := restarted.RecordManualBalance(ctx, f.request)
	if err != nil || !proto.Equal(first, replayed) {
		t.Fatal("restart hydrated later version/source/balance", replayed, err)
	}
	changed := proto.Clone(f.request).(*pb.RecordManualBalanceRequest)
	changed.Note = "other note"
	if _, err := client.RecordManualBalance(ctx, changed); status.Code(err) != codes.AlreadyExists {
		t.Fatal("key/body conflict accepted", err)
	}
	changed = proto.Clone(f.request).(*pb.RecordManualBalanceRequest)
	changed.IdempotencyKey = uuid.NewString()
	if _, err := client.RecordManualBalance(ctx, changed); status.Code(err) != codes.Aborted {
		t.Fatal("new stale key was accepted as semantic EOD replay", err)
	}
	for _, badContext := range []string{"CSRF", "revoked"} {
		changed = proto.Clone(f.request).(*pb.RecordManualBalanceRequest)
		if badContext == "CSRF" {
			changed.Context.CsrfToken = "invalid"
		} else {
			changed.Context.Browser.CookieToken = "revoked-session"
		}
		if _, err := restarted.RecordManualBalance(ctx, changed); status.Code(err) != codes.PermissionDenied {
			t.Fatal("replay bypassed Auth", err)
		}
	}
	if _, err := f.client(t, f.app, mtls.Auth).RecordManualBalance(ctx, f.request); status.Code(err) != codes.PermissionDenied {
		t.Fatal("Auth peer wrote finances", err)
	}
}

func TestManualBalanceRPCZeroImportedSourceAndConflicts(t *testing.T) {
	f := newManualBalanceFixture(t)
	client := f.client(t, f.app, mtls.API)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f.request.Balance.AmountMinor = 0
	first, err := client.RecordManualBalance(ctx, f.request)
	if err != nil || first.Account.Balance == nil || first.Account.Balance.Amount.AmountMinor != 0 {
		t.Fatal(first, err)
	}
	parsed, err := time.Parse(time.DateOnly, string(f.today))
	if err != nil {
		t.Fatal(err)
	}
	statementDate := domain.Date(parsed.AddDate(0, 0, -1).Format(time.DateOnly))
	if _, err := f.app.Ledger.RecordBalance(ctx, ledger.BalanceCommand{Scope: f.scope, Date: statementDate, Amount: rub(t, 8000),
		Source: domain.BalanceSource{Kind: domain.StatementEOD, Namespace: domain.Namespace{System: "dataset_v1", CompanyID: "C001"}}, ExpectedDataVersion: 2}); err != nil {
		t.Fatal(err)
	}
	changed := proto.Clone(f.request).(*pb.RecordManualBalanceRequest)
	changed.IdempotencyKey = uuid.NewString()
	changed.AsOf = string(f.today)
	changed.ExpectedDataVersion = 3
	changed.Balance.AmountMinor = -1
	newAnchor, err := client.RecordManualBalance(ctx, changed)
	if err != nil || newAnchor.Account.TransactionSource.GetMode() != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED || newAnchor.Account.TransactionSource.GetSourceCompanyId() != "C001" || newAnchor.Account.DataVersion != 4 || newAnchor.Account.Balance.Amount.AmountMinor != -1 {
		t.Fatal("manual EOD changed imported authority", newAnchor, err)
	}
	for name, edit := range map[string]func(*pb.RecordManualBalanceRequest){
		"same day different observation": func(r *pb.RecordManualBalanceRequest) { r.AsOf = string(f.today); r.Balance.AmountMinor = 99 },
		"statement day":                  func(r *pb.RecordManualBalanceRequest) { r.AsOf = string(statementDate); r.Balance.AmountMinor = 8000 },
		"future":                         func(r *pb.RecordManualBalanceRequest) { r.AsOf = parsed.AddDate(0, 0, 1).Format(time.DateOnly) },
		"wrong account":                  func(r *pb.RecordManualBalanceRequest) { r.Scope.AccountId = uuid.NewString() },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(f.request).(*pb.RecordManualBalanceRequest)
			r.IdempotencyKey = uuid.NewString()
			r.ExpectedDataVersion = 4
			edit(r)
			_, err := client.RecordManualBalance(ctx, r)
			want := codes.FailedPrecondition
			if name == "future" {
				want = codes.InvalidArgument
			}
			if name == "wrong account" {
				want = codes.NotFound
			}
			if status.Code(err) != want {
				t.Fatal(err)
			}
		})
	}
	replayed, err := client.RecordManualBalance(ctx, f.request)
	if err != nil || !proto.Equal(first, replayed) {
		t.Fatal("original zero/source snapshot changed", replayed, err)
	}
}
