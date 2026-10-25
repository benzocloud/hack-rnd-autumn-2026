//go:build integration

package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func manualTransactionRequest(f *manualBalanceFixture) *pb.CreateManualTransactionRequest {
	inn := "0012345678"
	return &pb.CreateManualTransactionRequest{Context: f.request.Context, Scope: f.request.Scope, IdempotencyKey: uuid.NewString(), ExpectedDataVersion: 1,
		OperationDate: f.request.AsOf, Direction: pb.Direction_DIRECTION_INFLOW, Amount: &pb.Money{AmountMinor: 12345, Currency: "RUB"}, CounterpartyInn: &inn,
		CounterpartyName: "ООО <Покупатель>", PaymentPurpose: "Оплата кофе"}
}

func TestManualTransactionRPCConcurrentReplayAndStoredTime(t *testing.T) {
	f := newManualBalanceFixture(t)
	client := f.client(t, f.app, mtls.API)
	request := manualTransactionRequest(f)
	categoryID := uuid.NewString()
	if _, err := f.app.db.ExecContext(t.Context(), "INSERT INTO core_categories(company_id,id,name) VALUES($1,$2,'Продажи')", f.scope.CompanyID, categoryID); err != nil {
		t.Fatal(err)
	}
	request.CategoryId = &categoryID
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	results, errs := make([]*pb.CreateManualTransactionResponse, 8), make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i], errs[i] = client.CreateManualTransaction(ctx, request) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || !proto.Equal(results[0], results[i]) {
			t.Fatal("retry changed full response", i, err)
		}
	}
	first := results[0]
	fact := first.Transaction
	if first.DataVersion != 2 || fact == nil || fact.CreatedAt == nil || fact.CreatedAt.CheckValid() != nil || fact.Version != 1 || fact.Amount.AmountMinor != 12345 || fact.GetCounterpartyInn() != "0012345678" || fact.GetCategoryId() != categoryID || fact.Source.SourceSystem != "manual_v1" || fact.Source.Kind != pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL || fact.Source.ExternalOperationId != nil || fact.Source.SourceCompanyId != nil {
		t.Fatal(first)
	}
	id, err := uuid.Parse(fact.Id)
	if err != nil || id.Version() != 7 {
		t.Fatal(fact.Id, err)
	}
	var storedAt time.Time
	if err := f.app.db.GetContext(ctx, &storedAt, "SELECT created_at FROM core_transactions WHERE company_id=$1 AND account_id=$2 AND id=$3", f.scope.CompanyID, f.scope.AccountID, id); err != nil || !storedAt.Equal(fact.CreatedAt.AsTime()) {
		t.Fatal(storedAt, err)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_command_results WHERE company_id=$1 AND operation='create_manual_transaction_v1'", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_balances WHERE company_id=$1", f.scope, 0)
	account, err := client.GetDefaultAccount(ctx, &pb.GetDefaultAccountRequest{CompanyId: f.scope.CompanyID.String(), Context: request.Context.Browser})
	if err != nil || account.Account.Balance != nil || account.Account.TransactionSource.GetMode() != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL {
		t.Fatal(account, err)
	}
	// Simulate a later category edit; an original create retry must preserve v1.
	if _, err := f.app.db.ExecContext(ctx, "UPDATE core_transactions SET category_id=NULL,version=2 WHERE company_id=$1 AND account_id=$2 AND id=$3", f.scope.CompanyID, f.scope.AccountID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Ledger.RecordBalance(ctx, ledger.BalanceCommand{Scope: f.scope, Date: f.today, Amount: rub(t, 0), Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "counted", ExpectedDataVersion: 2, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	second, _ := testApplication(t)
	replayed, err := f.client(t, second, mtls.API).CreateManualTransaction(ctx, request)
	if err != nil || !proto.Equal(first, replayed) {
		t.Fatal("reconnected retry hydrated mutable record", replayed, err)
	}
	changed := proto.Clone(request).(*pb.CreateManualTransactionRequest)
	changed.PaymentPurpose = "other"
	if _, err := client.CreateManualTransaction(ctx, changed); status.Code(err) != codes.AlreadyExists {
		t.Fatal("key/body conflict accepted", err)
	}
	changed = proto.Clone(request).(*pb.CreateManualTransactionRequest)
	changed.IdempotencyKey = uuid.NewString()
	if _, err := client.CreateManualTransaction(ctx, changed); status.Code(err) != codes.Aborted {
		t.Fatal("new stale key accepted", err)
	}
	for _, mode := range []string{"CSRF", "revoked"} {
		changed = proto.Clone(request).(*pb.CreateManualTransactionRequest)
		if mode == "CSRF" {
			changed.Context.CsrfToken = "invalid"
		} else {
			changed.Context.Browser.CookieToken = "revoked-session"
		}
		if _, err := client.CreateManualTransaction(ctx, changed); status.Code(err) != codes.PermissionDenied {
			t.Fatal("retry bypassed Auth", mode, err)
		}
	}
	if _, err := f.client(t, f.app, mtls.Auth).CreateManualTransaction(ctx, request); status.Code(err) != codes.PermissionDenied {
		t.Fatal("Auth peer wrote ledger", err)
	}
	if _, err := f.app.db.ExecContext(ctx, "UPDATE core_command_results SET result=jsonb_set(result,'{schema_version}','2') WHERE company_id=$1 AND operation='create_manual_transaction_v1'", f.scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateManualTransaction(ctx, request); status.Code(err) != codes.Unavailable {
		t.Fatal("corrupt receipt accepted", err)
	}
}

func TestManualTransactionRPCRejectsForeignCategoryAndAcceptsImportedHistory(t *testing.T) {
	f := newManualBalanceFixture(t)
	client := f.client(t, f.app, mtls.API)
	request := manualTransactionRequest(f)
	other := newManualBalanceFixture(t)
	foreignCategory := uuid.NewString()
	if _, err := other.app.db.ExecContext(t.Context(), "INSERT INTO core_categories(company_id,id,name) VALUES($1,$2,'Чужая')", other.scope.CompanyID, foreignCategory); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*pb.CreateManualTransactionRequest){
		"foreign category":   func(r *pb.CreateManualTransactionRequest) { r.CategoryId = &foreignCategory },
		"wrong account":      func(r *pb.CreateManualTransactionRequest) { r.Scope.AccountId = uuid.NewString() },
		"zero":               func(r *pb.CreateManualTransactionRequest) { r.Amount.AmountMinor = 0 },
		"bad INN":            func(r *pb.CreateManualTransactionRequest) { v := "123"; r.CounterpartyInn = &v },
		"empty optional INN": func(r *pb.CreateManualTransactionRequest) { v := ""; r.CounterpartyInn = &v },
		"future date": func(r *pb.CreateManualTransactionRequest) {
			r.OperationDate = time.Now().AddDate(0, 0, 2).Format(time.DateOnly)
		},
		"unknown direction": func(r *pb.CreateManualTransactionRequest) { r.Direction = pb.Direction_DIRECTION_UNSPECIFIED },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(request).(*pb.CreateManualTransactionRequest)
			edit(r)
			_, err := client.CreateManualTransaction(t.Context(), r)
			want := codes.InvalidArgument
			if name == "wrong account" {
				want = codes.NotFound
			}
			if status.Code(err) != want {
				t.Fatal(err)
			}
		})
	}
	assertRevision(t, f.app, f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 0)
	assertCount(t, f.app, "SELECT count(*) FROM core_command_results WHERE company_id=$1", f.scope, 0)
	if _, err := f.app.Ledger.RecordBalance(t.Context(), ledger.BalanceCommand{Scope: f.scope, Date: domain.Date(request.OperationDate), Amount: rub(t, 50), Source: domain.BalanceSource{Kind: domain.StatementEOD, Namespace: domain.Namespace{System: "dataset_v1", CompanyID: "C001"}}, ExpectedDataVersion: 1}); err != nil {
		t.Fatal(err)
	}
	request.ExpectedDataVersion = 2
	created, err := client.CreateManualTransaction(t.Context(), request)
	if err != nil || created.DataVersion != 3 || created.Transaction.Source.Kind != pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL {
		t.Fatal(created, err)
	}
	assertRevision(t, f.app, f.scope, 3)
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_command_results WHERE company_id=$1", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_accounts WHERE company_id=$1 AND factual_kind='imported' AND source_system='dataset_v1' AND source_company_id='C001'", f.scope, 1)
}
