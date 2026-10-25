package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (h *handler) ledgerRoute(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/companies/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/companies/"), "/")
	method := http.MethodGet
	var action http.HandlerFunc
	switch {
	case len(parts) == 2 && parts[1] == "categories":
		action = h.listCategories
	case len(parts) == 2 && parts[1] == "transactions":
		action = h.listTransactions
		if r.Method == http.MethodPost {
			method, action = http.MethodPost, h.createManualTransaction
		}
	case len(parts) == 4 && parts[1] == "transactions" && parts[3] == "category":
		method, action = http.MethodPatch, h.updateTransactionCategory
	default:
		return false
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		if len(parts) == 2 && parts[1] == "transactions" {
			w.Header().Set("Allow", "GET, POST")
		}
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !resourceID(parts[0]) || len(parts) == 4 && !resourceID(parts[2]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if (method == http.MethodPatch || method == http.MethodPost) && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	if h.core.Ledger == nil {
		dependencyError(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("company_id", parts[0])
	if len(parts) == 4 {
		r.SetPathValue("transaction_id", parts[2])
	}
	action(w, r)
	return true
}

func (h *handler) listCategories(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	response, err := h.core.Ledger.ListCategories(r.Context(), &corev1.ListCategoriesRequest{Context: browser(r), CompanyId: r.PathValue("company_id"), Page: page})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	seen := make(map[string]bool)
	for _, category := range response.Items {
		if category == nil || !resourceID(category.Id) || !nonempty(category.Name, 160) || !contains(category.SemanticKind, "operating_expense", "sales_receipt", "tax", "financing", "own_transfer", "other") || seen[category.Id] {
			dependencyError(w, r)
			return
		}
		var direction any
		if category.Direction != corev1.Direction_DIRECTION_UNSPECIFIED {
			name := directionName(category.Direction)
			if !contains(name, "inflow", "outflow") {
				dependencyError(w, r)
				return
			}
			direction = name
		}
		seen[category.Id] = true
		items = append(items, object{"id": category.Id, "name": category.Name, "direction": direction, "semantic_kind": category.SemanticKind})
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}

func (h *handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	request, ok := transactionQuery(r)
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
	response, err := h.core.Ledger.ListTransactions(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	seen := make(map[string]bool)
	for _, transaction := range response.Items {
		value, err := transactionDTO(transaction, account.Id)
		if err != nil || seen[transaction.Id] {
			dependencyError(w, r)
			return
		}
		seen[transaction.Id] = true
		items = append(items, value)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}

func transactionQuery(r *http.Request) (*corev1.ListTransactionsRequest, bool) {
	query, err := parseQuery(r)
	if err != nil {
		return nil, false
	}
	request := &corev1.ListTransactionsRequest{Page: &corev1.PageRequest{}, Dates: &corev1.DateRange{}}
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
		case "direction":
			direction := corev1.Direction_DIRECTION_INFLOW
			if value == "outflow" {
				direction = corev1.Direction_DIRECTION_OUTFLOW
			} else if value != "inflow" {
				return nil, false
			}
			request.Direction = &direction
		case "category_id":
			if !resourceID(value) {
				return nil, false
			}
			request.CategoryId = &value
		default:
			return nil, false
		}
	}
	if request.Dates.From != nil && request.Dates.To != nil && *request.Dates.From > *request.Dates.To {
		return nil, false
	}
	return request, true
}

func (h *handler) updateTransactionCategory(w http.ResponseWriter, r *http.Request) {
	fields, err := ledgerBody(w, r, "version", "category_id")
	if err != nil || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	version, ok := ledgerVersion(fields["version"])
	if !ok {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	request := &corev1.UpdateTransactionCategoryRequest{Context: writeContext(r), TransactionId: r.PathValue("transaction_id"), ExpectedVersion: version}
	if bytes.Equal(bytes.TrimSpace(fields["category_id"]), []byte("null")) {
		request.CategoryChange = &corev1.UpdateTransactionCategoryRequest_ClearCategory{ClearCategory: &emptypb.Empty{}}
	} else {
		var category string
		if json.Unmarshal(fields["category_id"], &category) != nil || !resourceID(category) {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
		request.CategoryChange = &corev1.UpdateTransactionCategoryRequest_CategoryId{CategoryId: category}
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Scope = scopeFor(account)
	response, err := h.core.Ledger.UpdateTransactionCategory(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := transactionDTO(response.GetTransaction(), account.Id)
	if err != nil || response.GetTransaction().GetId() != request.TransactionId || response.GetDataVersion() < 1 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
