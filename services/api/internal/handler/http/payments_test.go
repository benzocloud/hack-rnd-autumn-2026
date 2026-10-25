package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestPaymentCreateExactMoneyAndPresence(t *testing.T) {
	t.Parallel()
	base := `{"direction":"outflow","amount":{"amount_minor":"9223372036854775807","currency":"RUB"},"scheduled_date":"2026-10-25","counterparty_name":"","description":""}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"exact maximum", base, true},
		{"numeric minor", strings.Replace(base, `"9223372036854775807"`, `9223372036854775807`, 1), false},
		{"overflow", strings.Replace(base, `9223372036854775807`, `9223372036854775808`, 1), false},
		{"duplicate nested", strings.Replace(base, `"currency":"RUB"`, `"currency":"RUB","currency":"RUB"`, 1), false},
		{"unknown nested", strings.Replace(base, `"currency":"RUB"`, `"currency":"RUB","extra":true`, 1), false},
		{"null required", strings.Replace(base, `"counterparty_name":""`, `"counterparty_name":null`, 1), false},
		{"unknown root", strings.Replace(base, `"direction":"outflow"`, `"direction":"outflow","actor":"owner"`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			p, ok := paymentCreateBody(httptest.NewRecorder(), r)
			if ok != tc.valid {
				t.Fatalf("valid=%v want %v", ok, tc.valid)
			}
			if ok && p.Amount.AmountMinor != int64(9223372036854775807) {
				t.Fatal("amount narrowed")
			}
		})
	}
}
func TestPaymentPatchPreservesOmissionAndClearsExplicitNull(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body         string
		valid, clear bool
	}{
		{`{"version":"1"}`, false, false},
		{`{"version":"1","description":""}`, true, false},
		{`{"version":"1","category_id":null,"reconciliation_group_id":null}`, true, true},
		{`{"version":"1","scheduled_date":null}`, false, false},
		{`{"version":1,"description":"x"}`, false, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			p, ok := paymentUpdateBody(httptest.NewRecorder(), r)
			if ok != tc.valid {
				t.Fatalf("valid=%v", ok)
			}
			if ok && tc.clear {
				if _, ok := p.CategoryChange.(*corev1.UpdatePaymentRequest_ClearCategory); !ok {
					t.Fatal("category clear lost")
				}
				if _, ok := p.ReconciliationChange.(*corev1.UpdatePaymentRequest_ClearReconciliationGroup); !ok {
					t.Fatal("group clear lost")
				}
			}
		})
	}
}
func TestPaymentGroupsRequireExplicitTaggedEvidence(t *testing.T) {
	t.Parallel()
	base := `{"cutoff_date":"2026-10-25","horizon_start":"2026-10-26","horizon_end":"2026-11-24","kind":"additional","additional":{"confirmed":true}}`
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{base, true},
		{strings.Replace(base, `"confirmed":true`, `"confirmed":false`, 1), false},
		{strings.Replace(base, `"additional":{"confirmed":true}`, `"replacement":{"complete_for_horizon":true,"historical_operations":[]}`, 1), false},
		{strings.Replace(base, `"additional":{"confirmed":true}`, `"additional":{"confirmed":true,"actor":"other"}`, 1), false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			_, ok := paymentGroupBody(httptest.NewRecorder(), r)
			if ok != tc.valid {
				t.Fatalf("valid=%v want=%v", ok, tc.valid)
			}
		})
	}
}
func TestPaymentMatchHistoryPreservesRemovalTime(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	removed := created.Add(time.Minute)
	match := &corev1.PaymentMatch{Id: forecastID, PaymentId: companyID, TransactionId: accountID, Amount: rub(12), CreatedAt: timestamppb.New(created), RemovedAt: timestamppb.New(removed)}
	value, err := paymentMatchDTO(match, companyID)
	if err != nil || value["removed_at"] != removed.Format(time.RFC3339) {
		t.Fatalf("removed projection=%v err=%v", value, err)
	}
	match.RemovedAt = timestamppb.New(created.Add(-time.Minute))
	if _, err := paymentMatchDTO(match, companyID); err == nil {
		t.Fatal("accepted removal preceding creation")
	}
}

func TestPaymentProjectionRejectsMissingSource(t *testing.T) {
	t.Parallel()
	payment := &corev1.ExpectedPayment{Id: forecastID, AccountId: accountID, Direction: corev1.Direction_DIRECTION_OUTFLOW, Amount: rub(12), RemainingAmount: rub(12), ScheduledDate: "2026-10-25", Status: corev1.PaymentStatus_PAYMENT_STATUS_PLANNED, Version: 1, CreatedAt: timestamppb.Now()}
	if _, err := paymentDTO(payment, accountID); err == nil {
		t.Fatal("missing payment provenance accepted")
	}
}
