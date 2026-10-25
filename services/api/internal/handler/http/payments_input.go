package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func paymentBody(w http.ResponseWriter, r *http.Request, allowed []string, required ...string) (map[string]json.RawMessage, error) {
	return paymentFields(w, r, 16<<10, allowed, required...)
}
func paymentFields(w http.ResponseWriter, r *http.Request, limit int64, allowed []string, required ...string) (map[string]json.RawMessage, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, errors.New("JSON content type required")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil || !utf8.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return nil, errors.New("invalid JSON body")
	}
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return nil, errors.New("invalid JSON body")
	}
	for key := range fields {
		if !contains(key, allowed...) {
			return nil, errors.New("invalid JSON field")
		}
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return nil, errors.New("missing JSON field")
		}
	}
	return fields, nil
}
func paymentText(fields map[string]json.RawMessage, key string, maximum int) (*string, bool) {
	raw, ok := fields[key]
	if !ok {
		return nil, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !bounded(value, maximum) {
		return nil, false
	}
	return &value, true
}
func paymentMoney(raw json.RawMessage) (*corev1.Money, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 {
		return nil, false
	}
	minor, ok := ledgerVersion(fields["amount_minor"])
	if !ok {
		return nil, false
	}
	var currency string
	if json.Unmarshal(fields["currency"], &currency) != nil || currency != "RUB" {
		return nil, false
	}
	return &corev1.Money{AmountMinor: minor, Currency: currency}, true
}
func paymentOptionalID(fields map[string]json.RawMessage, key string) (*string, bool) {
	raw, ok := fields[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	var id string
	if json.Unmarshal(raw, &id) != nil || !resourceID(id) {
		return nil, false
	}
	return &id, true
}
func paymentCreateBody(w http.ResponseWriter, r *http.Request) (*corev1.CreateManualPaymentRequest, bool) {
	fields, err := paymentBody(w, r, []string{"direction", "amount", "scheduled_date", "counterparty_name", "description", "category_id", "reconciliation_group_id"}, "direction", "amount", "scheduled_date", "counterparty_name", "description")
	if err != nil {
		return nil, false
	}
	request := &corev1.CreateManualPaymentRequest{}
	direction, ok := paymentText(fields, "direction", 8)
	if !ok || direction == nil {
		return nil, false
	}
	switch *direction {
	case "inflow":
		request.Direction = corev1.Direction_DIRECTION_INFLOW
	case "outflow":
		request.Direction = corev1.Direction_DIRECTION_OUTFLOW
	default:
		return nil, false
	}
	request.Amount, ok = paymentMoney(fields["amount"])
	if !ok {
		return nil, false
	}
	date, ok := paymentText(fields, "scheduled_date", 10)
	if !ok || date == nil || !businessDate(*date) {
		return nil, false
	}
	request.ScheduledDate = *date
	name, ok := paymentText(fields, "counterparty_name", 500)
	if !ok || name == nil {
		return nil, false
	}
	request.CounterpartyName = *name
	description, ok := paymentText(fields, "description", 2000)
	if !ok || description == nil {
		return nil, false
	}
	request.Description = *description
	request.CategoryId, ok = paymentOptionalID(fields, "category_id")
	if !ok {
		return nil, false
	}
	request.ReconciliationGroupId, ok = paymentOptionalID(fields, "reconciliation_group_id")
	return request, ok
}
func paymentUpdateBody(w http.ResponseWriter, r *http.Request) (*corev1.UpdatePaymentRequest, bool) {
	fields, err := paymentBody(w, r, []string{"version", "direction", "amount", "scheduled_date", "counterparty_name", "description", "category_id", "reconciliation_group_id"}, "version")
	if err != nil || len(fields) < 2 {
		return nil, false
	}
	version, ok := ledgerVersion(fields["version"])
	if !ok {
		return nil, false
	}
	request := &corev1.UpdatePaymentRequest{ExpectedVersion: version}
	if _, present := fields["direction"]; present {
		direction, ok := paymentText(fields, "direction", 8)
		if !ok || direction == nil {
			return nil, false
		}
		d := corev1.Direction_DIRECTION_INFLOW
		if *direction == "outflow" {
			d = corev1.Direction_DIRECTION_OUTFLOW
		} else if *direction != "inflow" {
			return nil, false
		}
		request.Direction = &d
	}
	if raw, present := fields["amount"]; present {
		request.Amount, ok = paymentMoney(raw)
		if !ok {
			return nil, false
		}
	}
	request.ScheduledDate, ok = paymentText(fields, "scheduled_date", 10)
	if !ok || request.ScheduledDate != nil && !businessDate(*request.ScheduledDate) {
		return nil, false
	}
	request.CounterpartyName, ok = paymentText(fields, "counterparty_name", 500)
	if !ok {
		return nil, false
	}
	request.Description, ok = paymentText(fields, "description", 2000)
	if !ok {
		return nil, false
	}
	if _, present := fields["category_id"]; present {
		id, ok := paymentOptionalID(fields, "category_id")
		if !ok {
			return nil, false
		}
		if id == nil {
			request.CategoryChange = &corev1.UpdatePaymentRequest_ClearCategory{ClearCategory: &emptypb.Empty{}}
		} else {
			request.CategoryChange = &corev1.UpdatePaymentRequest_CategoryId{CategoryId: *id}
		}
	}
	if _, present := fields["reconciliation_group_id"]; present {
		id, ok := paymentOptionalID(fields, "reconciliation_group_id")
		if !ok {
			return nil, false
		}
		if id == nil {
			request.ReconciliationChange = &corev1.UpdatePaymentRequest_ClearReconciliationGroup{ClearReconciliationGroup: &emptypb.Empty{}}
		} else {
			request.ReconciliationChange = &corev1.UpdatePaymentRequest_ReconciliationGroupId{ReconciliationGroupId: *id}
		}
	}
	return request, true
}
func paymentMatchBody(w http.ResponseWriter, r *http.Request) (*corev1.MatchPaymentRequest, bool) {
	fields, err := paymentBody(w, r, []string{"transaction_id", "amount", "payment_version", "transaction_version"}, "transaction_id", "amount", "payment_version", "transaction_version")
	if err != nil {
		return nil, false
	}
	request := &corev1.MatchPaymentRequest{}
	transaction, ok := paymentOptionalID(fields, "transaction_id")
	if !ok || transaction == nil {
		return nil, false
	}
	request.TransactionId = *transaction
	request.Amount, ok = paymentMoney(fields["amount"])
	if !ok {
		return nil, false
	}
	request.ExpectedPaymentVersion, ok = ledgerVersion(fields["payment_version"])
	if !ok {
		return nil, false
	}
	request.ExpectedTransactionVersion, ok = ledgerVersion(fields["transaction_version"])
	return request, ok
}
