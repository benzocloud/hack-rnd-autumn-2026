package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (h *handler) createManualTransaction(w http.ResponseWriter, r *http.Request) {
	request, ok := manualTransactionInput(w, r)
	keys := r.Header.Values("Idempotency-Key")
	if !ok || r.URL.RawQuery != "" || len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope, request.IdempotencyKey = writeContext(r), scopeFor(account), keys[0]
	response, err := h.core.Ledger.CreateManualTransaction(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	t := response.GetTransaction()
	value, err := transactionDTO(t, account.Id)
	if err != nil || t.Version != 1 || t.CreatedAt == nil || t.Source.Kind != pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL || t.OperationDate != request.OperationDate || t.Direction != request.Direction || t.Amount.AmountMinor != request.Amount.AmountMinor || t.Amount.Currency != request.Amount.Currency || t.GetCounterpartyInn() != request.GetCounterpartyInn() || t.CounterpartyName != request.CounterpartyName || t.PaymentPurpose != request.PaymentPurpose || t.GetCategoryId() != request.GetCategoryId() || response.GetDataVersion() <= request.ExpectedDataVersion || response.GetDataVersion()-request.ExpectedDataVersion != 1 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func manualTransactionInput(w http.ResponseWriter, r *http.Request) (*pb.CreateManualTransactionRequest, bool) {
	fields, err := ledgerObject(w, r)
	if err != nil {
		return nil, false
	}
	for _, key := range []string{"operation_date", "direction", "amount", "counterparty_name", "payment_purpose", "expected_data_version"} {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	for key := range fields {
		if !contains(key, "operation_date", "direction", "amount", "counterparty_inn", "counterparty_name", "payment_purpose", "category_id", "expected_data_version") {
			return nil, false
		}
	}
	var date, direction, name, purpose string
	if json.Unmarshal(fields["operation_date"], &date) != nil || !businessDate(date) || json.Unmarshal(fields["direction"], &direction) != nil || !contains(direction, "inflow", "outflow") || bytes.Equal(bytes.TrimSpace(fields["counterparty_name"]), []byte("null")) || json.Unmarshal(fields["counterparty_name"], &name) != nil || !bounded(name, 500) || bytes.Equal(bytes.TrimSpace(fields["payment_purpose"]), []byte("null")) || json.Unmarshal(fields["payment_purpose"], &purpose) != nil || !bounded(purpose, 2000) {
		return nil, false
	}
	version, ok := ledgerVersion(fields["expected_data_version"])
	if !ok {
		return nil, false
	}
	var amount map[string]json.RawMessage
	if json.Unmarshal(fields["amount"], &amount) != nil || len(amount) != 2 {
		return nil, false
	}
	minor, ok := ledgerVersion(amount["amount_minor"])
	var currency string
	if !ok || json.Unmarshal(amount["currency"], &currency) != nil || currency != "RUB" {
		return nil, false
	}
	request := &pb.CreateManualTransactionRequest{OperationDate: date, ExpectedDataVersion: version, Direction: pb.Direction_DIRECTION_INFLOW, Amount: &pb.Money{AmountMinor: minor, Currency: currency}, CounterpartyName: name, PaymentPurpose: purpose}
	if direction == "outflow" {
		request.Direction = pb.Direction_DIRECTION_OUTFLOW
	}
	if raw, exists := fields["counterparty_inn"]; exists {
		if json.Unmarshal(raw, &request.CounterpartyInn) != nil || request.CounterpartyInn != nil && !innPattern.MatchString(*request.CounterpartyInn) {
			return nil, false
		}
	}
	if raw, exists := fields["category_id"]; exists {
		if json.Unmarshal(raw, &request.CategoryId) != nil || !optionalID(request.CategoryId) {
			return nil, false
		}
	}
	return request, true
}
