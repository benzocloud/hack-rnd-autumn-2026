package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (h *handler) paymentRoute(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/api/v1/companies/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 2 || parts[1] != "payments" {
		return false
	}
	var action http.HandlerFunc
	allow := ""
	switch {
	case len(parts) == 2:
		allow = "GET, POST"
		switch r.Method {
		case http.MethodGet:
			action = h.listPayments
		case http.MethodPost:
			action = h.createPayment
		}
	case len(parts) == 3:
		allow = "GET, PATCH, DELETE"
		switch r.Method {
		case http.MethodGet:
			action = h.getPayment
		case http.MethodPatch:
			action = h.updatePayment
		case http.MethodDelete:
			action = h.cancelPayment
		}
	case len(parts) == 4 && parts[3] == "matches":
		allow = "GET, POST"
		switch r.Method {
		case http.MethodGet:
			action = h.listPaymentMatches
		case http.MethodPost:
			action = h.matchPayment
		}
	case len(parts) == 5 && parts[3] == "matches":
		allow = "DELETE"
		if r.Method == http.MethodDelete {
			action = h.removePaymentMatch
		}
	default:
		return false
	}
	if action == nil {
		w.Header().Set("Allow", allow)
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !resourceID(parts[0]) || len(parts) > 2 && !resourceID(parts[2]) || len(parts) == 5 && !resourceID(parts[4]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if r.Method != http.MethodGet && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	if h.core.Payments == nil || h.core.Ledger == nil {
		dependencyError(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("company_id", parts[0])
	if len(parts) > 2 {
		r.SetPathValue("payment_id", parts[2])
	}
	if len(parts) == 5 {
		r.SetPathValue("match_id", parts[4])
	}
	action(w, r)
	return true
}
func (h *handler) listPayments(w http.ResponseWriter, r *http.Request) {
	request, ok := paymentQuery(r)
	if !ok {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope = browser(r), scopeFor(account)
	response, err := h.core.Payments.ListPayments(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	seen := map[string]bool{}
	for _, p := range response.Items {
		value, err := paymentDTO(p, account.Id)
		if err != nil || seen[p.Id] {
			dependencyError(w, r)
			return
		}
		seen[p.Id] = true
		items = append(items, value)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}
func paymentQuery(r *http.Request) (*corev1.ListPaymentsRequest, bool) {
	query, err := parseQuery(r)
	if err != nil {
		return nil, false
	}
	request := &corev1.ListPaymentsRequest{Page: &corev1.PageRequest{}, Dates: &corev1.DateRange{}}
	for key, values := range query {
		if len(values) != 1 {
			return nil, false
		}
		value := values[0]
		switch key {
		case "limit":
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil || n < 1 || n > 100 || strconv.FormatUint(n, 10) != value {
				return nil, false
			}
			limit := uint32(n)
			request.Page.Limit = &limit
		case "cursor":
			if !validCursor(value) {
				return nil, false
			}
			request.Page.Cursor = value
		case "from", "to":
			if !businessDate(value) {
				return nil, false
			}
			if key == "from" {
				request.Dates.From = &value
			} else {
				request.Dates.To = &value
			}
		case "status":
			status, ok := paymentStatus(value)
			if !ok {
				return nil, false
			}
			request.Status = &status
		default:
			return nil, false
		}
	}
	if request.Dates.From != nil && request.Dates.To != nil && *request.Dates.From > *request.Dates.To {
		return nil, false
	}
	return request, true
}
func (h *handler) getPayment(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.GetPayment(r.Context(), &corev1.GetPaymentRequest{Context: browser(r), Scope: scopeFor(account), PaymentId: r.PathValue("payment_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentDTO(response.GetPayment(), account.Id)
	if err != nil || response.GetPayment().GetId() != r.PathValue("payment_id") {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
func (h *handler) createPayment(w http.ResponseWriter, r *http.Request) {
	request, ok := paymentCreateBody(w, r)
	keys := r.Header.Values("Idempotency-Key")
	if !ok || len(keys) != 1 || !validIdempotencyKey(keys[0]) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope, request.IdempotencyKey = writeContext(r), scopeFor(account), keys[0]
	response, err := h.core.Payments.CreateManualPayment(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentDTO(response.GetPayment(), account.Id)
	if err != nil || response.GetDataVersion() < 1 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 201, value)
}
func (h *handler) updatePayment(w http.ResponseWriter, r *http.Request) {
	request, ok := paymentUpdateBody(w, r)
	if !ok || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope, request.PaymentId = writeContext(r), scopeFor(account), r.PathValue("payment_id")
	response, err := h.core.Payments.UpdatePayment(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentDTO(response.GetPayment(), account.Id)
	if err != nil || response.GetDataVersion() < 1 || response.GetPayment().GetId() != request.PaymentId {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
func deleteVersion(r *http.Request, key string) (int64, bool) {
	q, err := parseQuery(r)
	if err != nil || len(q) != 1 || len(q[key]) != 1 {
		return 0, false
	}
	value := q.Get(key)
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}
func (h *handler) cancelPayment(w http.ResponseWriter, r *http.Request) {
	version, ok := deleteVersion(r, "version")
	if !ok {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.CancelPayment(r.Context(), &corev1.CancelPaymentRequest{Context: writeContext(r), Scope: scopeFor(account), PaymentId: r.PathValue("payment_id"), ExpectedVersion: version})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if _, err := paymentDTO(response.GetPayment(), account.Id); err != nil || response.GetDataVersion() < 1 || response.GetPayment().GetId() != r.PathValue("payment_id") {
		dependencyError(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *handler) listPaymentMatches(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.ListPaymentMatches(r.Context(), &corev1.ListPaymentMatchesRequest{Context: browser(r), Scope: scopeFor(account), PaymentId: r.PathValue("payment_id"), Page: page})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	seen := map[string]bool{}
	for _, m := range response.Items {
		value, err := paymentMatchDTO(m, r.PathValue("payment_id"))
		if err != nil || seen[m.Id] {
			dependencyError(w, r)
			return
		}
		seen[m.Id] = true
		items = append(items, value)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}
func (h *handler) matchPayment(w http.ResponseWriter, r *http.Request) {
	request, ok := paymentMatchBody(w, r)
	keys := r.Header.Values("Idempotency-Key")
	if !ok || len(keys) != 1 || !validIdempotencyKey(keys[0]) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope, request.PaymentId, request.IdempotencyKey = writeContext(r), scopeFor(account), r.PathValue("payment_id"), keys[0]
	response, err := h.core.Payments.MatchPayment(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentMatchDTO(response.GetMatch(), request.PaymentId)
	if err != nil || response.GetDataVersion() < 1 || response.GetPaymentVersion() < 1 || response.GetMatch().GetTransactionId() != request.TransactionId {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 201, value)
}
func (h *handler) removePaymentMatch(w http.ResponseWriter, r *http.Request) {
	version, ok := deleteVersion(r, "payment_version")
	if !ok {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.RemovePaymentMatch(r.Context(), &corev1.RemovePaymentMatchRequest{Context: writeContext(r), Scope: scopeFor(account), PaymentId: r.PathValue("payment_id"), MatchId: r.PathValue("match_id"), ExpectedPaymentVersion: version})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if _, err := paymentDTO(response.GetPayment(), account.Id); err != nil || response.GetDataVersion() < 1 || response.GetPayment().GetId() != r.PathValue("payment_id") {
		dependencyError(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
