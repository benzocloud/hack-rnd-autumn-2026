//go:build mlintegration

package ml

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	forecastDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	paymentDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func TestPythonFinancialPredictMTLS(t *testing.T) {
	python, source := os.Getenv("CORE_ML_TEST_PYTHON"), os.Getenv("CORE_ML_TEST_SOURCE")
	if python == "" || source == "" {
		t.Skip("set CORE_ML_TEST_PYTHON and CORE_ML_TEST_SOURCE")
	}
	tlsConfig, certs := testTLS(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	script := `import pathlib, sys
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
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("ML did not start", scanner.Err())
	}
	port, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid ML port", scanner.Text())
	}
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := New(conn, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	csvResult, err := client.PredictFinancial(ctx, csvFixture(t))
	if err != nil || len(csvResult.Projection.Days) != 30 || csvResult.Projection.Days[29].ReferenceBalance.Minor() != 97000 || csvResult.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal("CSV profile", csvResult, err)
	}
	mixed := csvFixture(t)
	manual := mixed.Operations[1].Data()
	manual.Source = ledger.Source{Kind: ledger.Manual}
	mixed.Operations[1], err = ledger.NewTransaction(manual)
	if err != nil {
		t.Fatal(err)
	}
	mixedResult, err := client.PredictFinancial(ctx, mixed)
	if err != nil || len(mixedResult.Projection.Days) != 30 || mixedResult.Projection.Days[29].ReferenceBalance.Minor() != 97000 || mixedResult.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal("CSV with manual supplement", mixedResult, err)
	}
	iCurrent := financialFixture(t)
	cutoff, _ := time.Parse(time.DateOnly, string(iCurrent.Snapshot.Cutoff))
	group := forecastDomain.ReconciliationGroup{ID: uuid.New(), Scope: iCurrent.Snapshot.Scope, Version: 1, Status: "active", Cutoff: iCurrent.Snapshot.Cutoff, HorizonStart: ledger.Date(cutoff.AddDate(0, 0, 1).Format(time.DateOnly)), HorizonEnd: ledger.Date(cutoff.AddDate(0, 0, 30).Format(time.DateOnly)), ConfirmedBy: "test-actor", ConfirmedAt: time.Now().UTC(), Additional: true}
	iCurrent.Calendar = forecastDomain.Calendar{Captured: true, KnowledgeMode: "current", AsOf: iCurrent.Snapshot.Cutoff, NoPostCutoffMovements: true, Groups: []forecastDomain.ReconciliationGroup{group}}
	currentBase, err := client.PredictFinancial(ctx, iCurrent)
	if err != nil || currentBase.Projection.HistoryOnlyIncludesRecurringFlows {
		t.Fatal("available empty calendar", err)
	}
	payment, err := paymentDomain.NewExpectedPayment(paymentDomain.PaymentData{ID: uuid.New(), ReconciliationGroupID: group.ID, Scope: iCurrent.Snapshot.Scope, Direction: ledger.Outflow, Amount: rub(5000), ScheduledDate: ledger.Date(cutoff.AddDate(0, 0, 2).Format(time.DateOnly)), Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta := func(label string, p paymentDomain.ExpectedPayment, amount int64) {
		t.Helper()
		iCurrent.Calendar.Payments = []paymentDomain.ExpectedPayment{p}
		result, err := client.PredictFinancial(ctx, iCurrent)
		if err != nil || len(result.Projection.Days) != 30 {
			t.Fatal(label, err)
		}
		for n, day := range result.Projection.Days {
			expected := currentBase.Projection.Days[n].ReferenceBalance.Minor()
			if day.Date >= p.Data().ScheduledDate {
				expected -= amount
			}
			if day.ReferenceBalance.Minor() != expected {
				t.Fatalf("%s day %d: %d want %d", label, n, day.ReferenceBalance.Minor(), expected)
			}
		}
	}
	assertPaymentDelta("created", payment, 5000)
	editedAmount := rub(7000)
	payment, err = payment.Update(paymentDomain.PaymentChange{Amount: &editedAmount})
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta("amount edit", payment, 7000)
	editedDate := ledger.Date(cutoff.AddDate(0, 0, 3).Format(time.DateOnly))
	payment, err = payment.Update(paymentDomain.PaymentChange{ScheduledDate: &editedDate})
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta("date edit", payment, 7000)
	fact := iCurrent.Operations[1]
	partial, match, err := payment.Match(uuid.New(), fact, rub(0), rub(100), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta("partial match", partial, 6900)
	unmatched, err := partial.RemoveMatch(match)
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta("unmatch", unmatched, 7000)
	cancelled, err := partial.Cancel()
	if err != nil {
		t.Fatal(err)
	}
	assertPaymentDelta("partial cancel", cancelled, 0)
	// Complete replacement survives zero active occurrences. The factual operation
	// remains in history/EOD and is excluded only from residual inference.
	replacement := group
	replacement.Additional = false
	replacement.Operations = []forecastDomain.OperationReference{{ID: fact.Data().ID, Version: fact.Version()}}
	iCurrent.Calendar.Groups = []forecastDomain.ReconciliationGroup{replacement}
	iCurrent.Calendar.Payments = []paymentDomain.ExpectedPayment{cancelled}
	replaced, err := client.PredictFinancial(ctx, iCurrent)
	if err != nil {
		t.Fatal("replacement after cancel", err)
	}
	if replaced.Projection.Days[29].ReferenceBalance.Minor() != currentBase.Projection.Days[29].ReferenceBalance.Minor()+60 {
		t.Fatal("cancel revoked independent replacement", replaced.Projection.Days[29])
	}
	beyond := ledger.Date(cutoff.AddDate(0, 0, 31).Format(time.DateOnly))
	moved, err := payment.Update(paymentDomain.PaymentChange{ScheduledDate: &beyond})
	if err != nil {
		t.Fatal(err)
	}
	iCurrent.Calendar.Payments = []paymentDomain.ExpectedPayment{moved}
	outside, err := client.PredictFinancial(ctx, iCurrent)
	if err != nil || outside.Projection.Days[29].ReferenceBalance != replaced.Projection.Days[29].ReferenceBalance {
		t.Fatal("out-of-horizon mapping lifetime", err)
	}
	fullAmount := rub(200)
	full, err := payment.Update(paymentDomain.PaymentChange{Amount: &fullAmount})
	if err != nil {
		t.Fatal(err)
	}
	full, _, err = full.Match(uuid.New(), fact, rub(0), rub(200), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	iCurrent.Calendar.Payments = []paymentDomain.ExpectedPayment{full}
	settled, err := client.PredictFinancial(ctx, iCurrent)
	if err != nil || settled.Projection.Days[29].ReferenceBalance != replaced.Projection.Days[29].ReferenceBalance {
		t.Fatal("full match mapping lifetime", err)
	}

	i := financialFixture(t)
	result, err := client.PredictFinancial(ctx, i)
	if err != nil || len(result.Projection.Days) != 30 || result.Projection.Days[29].ReferenceBalance.Minor() != -2700 || len(result.Components) != 30 || result.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal(result, err)
	}
	// Reconstructed days preserve provenance through the actual financial model.
	i.Snapshot.History[0].Source = "reconstructed_covered"
	if _, err := client.PredictFinancial(ctx, i); err != nil {
		t.Fatal("reconstructed financial history", err)
	}
	// Covered zero activity with an observed zero EOD is valid; a missing day
	// would not prove inactivity and must never take this path.
	i.Operations = nil
	for n := range i.Snapshot.History {
		i.Snapshot.History[n].Inflow = rub(0)
		i.Snapshot.History[n].Outflow = rub(0)
		i.Snapshot.History[n].ClosingBalance = rub(0)
		i.Snapshot.History[n].Source = forecasting.VerifiedNoOperations
	}
	i.Snapshot.History[89].Source = forecasting.Observed
	i.Snapshot.OpeningBalance = rub(0)
	zero, err := client.PredictFinancial(ctx, i)
	if err != nil || zero.Projection.FirstNegativeDate != nil || len(zero.Components) != 0 || zero.Projection.Days[29].ReferenceBalance.Minor() != 0 || zero.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal("covered zero history", zero, err)
	}
}
