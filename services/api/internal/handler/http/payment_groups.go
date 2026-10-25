package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (h *handler) paymentGroupRoute(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/api/v1/companies/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 2 || parts[1] != "reconciliation-groups" {
		return false
	}
	var action http.HandlerFunc
	allow := "GET"
	switch {
	case len(parts) == 2:
		allow = "GET, POST"
		if r.Method == http.MethodGet {
			action = h.listPaymentGroups
		}
		if r.Method == http.MethodPost {
			action = h.createPaymentGroup
		}
	case len(parts) == 3:
		if r.Method == http.MethodGet {
			action = h.getPaymentGroup
		}
	case len(parts) == 4 && parts[3] == "revoke":
		allow = "POST"
		if r.Method == http.MethodPost {
			action = h.revokePaymentGroup
		}
	default:
		return false
	}
	if action == nil {
		w.Header().Set("Allow", allow)
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !resourceID(parts[0]) || len(parts) > 2 && !resourceID(parts[2]) {
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
		r.SetPathValue("group_id", parts[2])
	}
	action(w, r)
	return true
}
func (h *handler) listPaymentGroups(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	request := &corev1.ListReconciliationGroupsRequest{Page: &corev1.PageRequest{}}
	for key, values := range query {
		if len(values) != 1 {
			writeError(w, r, 400, "INVALID_REQUEST", nil)
			return
		}
		value := values[0]
		switch key {
		case "cursor":
			if !validCursor(value) {
				writeError(w, r, 400, "INVALID_REQUEST", nil)
				return
			}
			request.Page.Cursor = value
		case "limit":
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil || n < 1 || n > 100 || strconv.FormatUint(n, 10) != value {
				writeError(w, r, 400, "INVALID_REQUEST", nil)
				return
			}
			limit := uint32(n)
			request.Page.Limit = &limit
		case "include_revoked":
			if value != "true" && value != "false" {
				writeError(w, r, 400, "INVALID_REQUEST", nil)
				return
			}
			request.IncludeRevoked = value == "true"
		default:
			writeError(w, r, 400, "INVALID_REQUEST", nil)
			return
		}
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context, request.Scope = browser(r), scopeFor(account)
	response, err := h.core.Payments.ListReconciliationGroups(r.Context(), request)
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
	for _, g := range response.Items {
		value, err := paymentGroupDTO(g, account.CompanyId, account.Id)
		if err != nil || seen[g.Id] {
			dependencyError(w, r)
			return
		}
		seen[g.Id] = true
		items = append(items, value)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}
func (h *handler) getPaymentGroup(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.GetReconciliationGroup(r.Context(), &corev1.GetReconciliationGroupRequest{Context: browser(r), Scope: scopeFor(account), GroupId: r.PathValue("group_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentGroupDTO(response.GetGroup(), account.CompanyId, account.Id)
	if err != nil || response.GetGroup().GetId() != r.PathValue("group_id") {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
func (h *handler) createPaymentGroup(w http.ResponseWriter, r *http.Request) {
	request, ok := paymentGroupBody(w, r)
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
	response, err := h.core.Payments.CreateReconciliationGroup(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentGroupDTO(response.GetGroup(), account.CompanyId, account.Id)
	if err != nil || response.GetDataVersion() < 1 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 201, value)
}
func (h *handler) revokePaymentGroup(w http.ResponseWriter, r *http.Request) {
	fields, err := paymentBody(w, r, []string{"version"}, "version")
	keys := r.Header.Values("Idempotency-Key")
	version, ok := ledgerVersion(fields["version"])
	if err != nil || !ok || r.URL.RawQuery != "" || len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Payments.RevokeReconciliationGroup(r.Context(), &corev1.RevokeReconciliationGroupRequest{Context: writeContext(r), Scope: scopeFor(account), GroupId: r.PathValue("group_id"), ExpectedVersion: version, IdempotencyKey: keys[0]})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := paymentGroupDTO(response.GetGroup(), account.CompanyId, account.Id)
	if err != nil || response.GetDataVersion() < 1 || response.GetGroup().GetId() != r.PathValue("group_id") {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
func paymentGroupBody(w http.ResponseWriter, r *http.Request) (*corev1.CreateReconciliationGroupRequest, bool) {
	fields, err := paymentFields(w, r, 2<<20, []string{"cutoff_date", "horizon_start", "horizon_end", "kind", "additional", "replacement"}, "cutoff_date", "horizon_start", "horizon_end", "kind")
	if err != nil || len(fields) != 5 {
		return nil, false
	}
	request := &corev1.CreateReconciliationGroupRequest{}
	cutoff, ok := paymentText(fields, "cutoff_date", 10)
	if !ok || cutoff == nil || !businessDate(*cutoff) {
		return nil, false
	}
	request.CutoffDate = *cutoff
	start, ok := paymentText(fields, "horizon_start", 10)
	if !ok || start == nil || !businessDate(*start) {
		return nil, false
	}
	request.HorizonStart = *start
	end, ok := paymentText(fields, "horizon_end", 10)
	if !ok || end == nil || !businessDate(*end) {
		return nil, false
	}
	request.HorizonEnd = *end
	kind, ok := paymentText(fields, "kind", 16)
	if !ok || kind == nil {
		return nil, false
	}
	switch *kind {
	case "additional":
		var rec map[string]json.RawMessage
		if json.Unmarshal(fields["additional"], &rec) != nil || len(rec) != 1 || !bytes.Equal(bytes.TrimSpace(rec["confirmed"]), []byte("true")) {
			return nil, false
		}
		request.Reconciliation = &corev1.CreateReconciliationGroupRequest_Additional{Additional: &corev1.AdditionalObligationConfirmation{Confirmed: true}}
	case "replacement":
		var rec map[string]json.RawMessage
		if json.Unmarshal(fields["replacement"], &rec) != nil || len(rec) != 2 || !bytes.Equal(bytes.TrimSpace(rec["complete_for_horizon"]), []byte("true")) {
			return nil, false
		}
		var refs []map[string]json.RawMessage
		if json.Unmarshal(rec["historical_operations"], &refs) != nil || len(refs) < 1 || len(refs) > 20000 {
			return nil, false
		}
		replacement := &corev1.CompleteReplacement{CompleteForHorizon: true}
		seen := map[string]bool{}
		for _, ref := range refs {
			id, ok := paymentOptionalID(ref, "transaction_id")
			version, versionOK := ledgerVersion(ref["version"])
			if len(ref) != 2 || !ok || id == nil || !versionOK || seen[*id] {
				return nil, false
			}
			seen[*id] = true
			replacement.HistoricalOperations = append(replacement.HistoricalOperations, &corev1.HistoricalOperationRef{TransactionId: *id, Version: version})
		}
		request.Reconciliation = &corev1.CreateReconciliationGroupRequest_Replacement{Replacement: replacement}
	default:
		return nil, false
	}
	return request, true
}
func paymentGroupDTO(g *corev1.PaymentReconciliationGroup, company, account string) (object, error) {
	if g == nil || !resourceID(g.Id) || g.Scope == nil || g.Scope.CompanyId != company || g.Scope.AccountId != account || g.Version < 1 || !businessDate(g.CutoffDate) || !businessDate(g.HorizonStart) || !businessDate(g.HorizonEnd) || !nonempty(g.ConfirmedBy, 128) || !timestamp(g.ConfirmedAt) {
		return nil, errProjection
	}
	cutoff, _ := time.Parse(time.DateOnly, g.CutoffDate)
	if cutoff.AddDate(0, 0, 1).Format(time.DateOnly) != g.HorizonStart || cutoff.AddDate(0, 0, 30).Format(time.DateOnly) != g.HorizonEnd {
		return nil, errProjection
	}
	status := ""
	switch g.Status {
	case corev1.ReconciliationStatus_RECONCILIATION_STATUS_ACTIVE:
		status = "active"
	case corev1.ReconciliationStatus_RECONCILIATION_STATUS_REVOKED:
		status = "revoked"
	case corev1.ReconciliationStatus_RECONCILIATION_STATUS_UNSPECIFIED:
		return nil, errProjection
	default:
		return nil, errProjection
	}
	out := object{"id": g.Id, "account_id": account, "version": strconv.FormatInt(g.Version, 10), "status": status, "cutoff_date": g.CutoffDate, "horizon_start": g.HorizonStart, "horizon_end": g.HorizonEnd, "confirmed_by": g.ConfirmedBy, "confirmed_at": utc(g.ConfirmedAt)}
	switch rec := g.Reconciliation.(type) {
	case *corev1.PaymentReconciliationGroup_Additional:
		if rec.Additional == nil || !rec.Additional.Confirmed {
			return nil, errProjection
		}
		out["kind"] = "additional"
		out["additional"] = object{"confirmed": true}
	case *corev1.PaymentReconciliationGroup_Replacement:
		if rec.Replacement == nil || !rec.Replacement.CompleteForHorizon || len(rec.Replacement.HistoricalOperations) == 0 || len(rec.Replacement.HistoricalOperations) > 20000 {
			return nil, errProjection
		}
		refs := make([]object, 0, len(rec.Replacement.HistoricalOperations))
		seen := map[string]bool{}
		for _, ref := range rec.Replacement.HistoricalOperations {
			if ref == nil || !resourceID(ref.TransactionId) || ref.Version < 1 || seen[ref.TransactionId] {
				return nil, errProjection
			}
			seen[ref.TransactionId] = true
			refs = append(refs, object{"transaction_id": ref.TransactionId, "version": strconv.FormatInt(ref.Version, 10)})
		}
		out["kind"] = "replacement"
		out["replacement"] = object{"complete_for_horizon": true, "historical_operations": refs}
	default:
		return nil, errProjection
	}
	return out, nil
}
