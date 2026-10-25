package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type createForecastRequest struct {
	Cutoff       string  `json:"cutoff_date"`
	Horizon      uint32  `json:"horizon_days"`
	CalendarMode *string `json:"calendar_mode"`
}

func (h *handler) financialRoute(w http.ResponseWriter, r *http.Request) bool {
	if h.paymentGroupRoute(w, r) {
		return true
	}
	if h.paymentRoute(w, r) {
		return true
	}
	if h.ledgerRoute(w, r) {
		return true
	}
	if h.importRoute(w, r) {
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	if parts[0] != "companies" {
		return false
	}
	method := http.MethodGet
	var action func(http.ResponseWriter, *http.Request)
	switch {
	case len(parts) == 1:
		action = h.listCompanies
	case len(parts) == 2:
		action = h.getCompany
	case len(parts) == 3 && parts[2] == "account":
		action = h.getAccount
	case len(parts) == 3 && parts[2] == "forecasts":
		action = h.listForecasts
		if r.Method == http.MethodPost {
			method, action = http.MethodPost, h.createForecast
		}
	case len(parts) == 4 && parts[2] == "forecasts":
		action = h.getForecast
	default:
		return false
	}
	if r.Method != method {
		if len(parts) == 3 && parts[2] == "forecasts" {
			w.Header().Set("Allow", "GET, POST")
		} else {
			w.Header().Set("Allow", method)
		}
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if len(parts) > 1 && !resourceID(parts[1]) || len(parts) == 4 && !resourceID(parts[3]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if method == http.MethodPost && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	if h.core.Companies == nil || h.core.Ledger == nil || h.core.Forecasts == nil {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if len(parts) > 1 {
		r.SetPathValue("company_id", parts[1])
	}
	if len(parts) == 4 {
		r.SetPathValue("forecast_id", parts[3])
	}
	action(w, r)
	return true
}

func readPage(r *http.Request, forecasts bool) (*corev1.PageRequest, bool, error) {
	q, err := parseQuery(r)
	if err != nil {
		return nil, false, err
	}
	page := &corev1.PageRequest{}
	include := false
	for key, values := range q {
		if len(values) != 1 {
			return nil, false, errors.New("duplicate query")
		}
		value := values[0]
		switch key {
		case "limit":
			n, e := strconv.ParseUint(value, 10, 32)
			if e != nil || n < 1 || n > 100 || strconv.FormatUint(n, 10) != value {
				return nil, false, errors.New("invalid limit")
			}
			limit := uint32(n)
			page.Limit = &limit
		case "cursor":
			if !validCursor(value) {
				return nil, false, errors.New("invalid cursor")
			}
			page.Cursor = value
		case "include_scenarios":
			if !forecasts || (value != "true" && value != "false") {
				return nil, false, errors.New("invalid include_scenarios")
			}
			include = value == "true"
		default:
			return nil, false, errors.New("unknown query")
		}
	}
	return page, include, nil
}

func (h *handler) listCompanies(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	memberships, err := h.auth.ListMemberships(r.Context(), &authv1.ListMembershipsRequest{Context: browser(r), Page: &authv1.PageRequest{Limit: page.Limit, Cursor: page.Cursor}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	if memberships == nil || memberships.Pagination == nil || !validCursor(memberships.Pagination.NextCursor) || len(memberships.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(memberships.Items))
	seen := make(map[string]bool)
	for _, m := range memberships.Items {
		if m == nil || !resourceID(m.CompanyId) || roleName(m.Role) == "" || seen[m.CompanyId] {
			dependencyError(w, r)
			return
		}
		seen[m.CompanyId] = true
		result, callErr := h.core.Companies.ReadCompany(r.Context(), &corev1.ReadCompanyRequest{Context: browser(r), CompanyId: m.CompanyId})
		if callErr != nil {
			coreRPCError(w, r, callErr)
			return
		}
		company, mapErr := companyDTO(result, m.CompanyId)
		if mapErr != nil || result.Role != m.Role {
			dependencyError(w, r)
			return
		}
		items = append(items, company)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(memberships.Pagination.NextCursor)})
}

func (h *handler) getCompany(w http.ResponseWriter, r *http.Request) {
	result, err := h.core.Companies.ReadCompany(r.Context(), &corev1.ReadCompanyRequest{Context: browser(r), CompanyId: r.PathValue("company_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := companyDTO(result, r.PathValue("company_id"))
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}

func (h *handler) accountScope(r *http.Request) (*corev1.Account, error) {
	result, err := h.core.Ledger.GetDefaultAccount(r.Context(), &corev1.GetDefaultAccountRequest{Context: browser(r), CompanyId: r.PathValue("company_id")})
	if err != nil {
		return nil, err
	}
	if result == nil || !validAccount(result.Account, r.PathValue("company_id")) {
		return nil, errProjection
	}
	return result.Account, nil
}
func (h *handler) getAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	writeJSON(w, 200, accountDTO(account))
}
func (h *handler) listForecasts(w http.ResponseWriter, r *http.Request) {
	page, include, err := readPage(r, true)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	result, err := h.core.Forecasts.ListForecasts(r.Context(), &corev1.ListForecastsRequest{Context: browser(r), Scope: scopeFor(account), Page: page, IncludeScenarios: include})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if result == nil || result.Pagination == nil || !validCursor(result.Pagination.NextCursor) || len(result.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(result.Items))
	for _, f := range result.Items {
		if !validSummary(f) || (!include && f.ScenarioId != nil) {
			dependencyError(w, r)
			return
		}
		items = append(items, summaryDTO(f))
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(result.Pagination.NextCursor)})
}
func (h *handler) createForecast(w http.ResponseWriter, r *http.Request) {
	var body createForecastRequest
	if err := decode(w, r, &body); err != nil || !businessDate(body.Cutoff) || body.Horizon < 30 || body.Horizon > 90 {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	mode := corev1.ForecastCalendarMode_FORECAST_CALENDAR_MODE_AVAILABLE
	if body.CalendarMode != nil {
		switch *body.CalendarMode {
		case "available":
		case "synthetic_scenario":
			mode = corev1.ForecastCalendarMode_FORECAST_CALENDAR_MODE_SYNTHETIC_SCENARIO
		default:
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	result, err := h.core.Forecasts.CreateForecast(r.Context(), &corev1.CreateForecastRequest{Context: writeContext(r), Scope: scopeFor(account), IdempotencyKey: keys[0], CutoffDate: body.Cutoff, HorizonDays: body.Horizon, CalendarMode: mode})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	accepted := result.GetAccepted()
	if accepted == nil || !resourceID(accepted.Id) || accepted.Status != corev1.ForecastStatus_FORECAST_STATUS_QUEUED || accepted.PollAfterMs < 1000 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 202, object{"id": accepted.Id, "status": "queued", "status_url": "/api/v1/companies/" + account.CompanyId + "/forecasts/" + accepted.Id, "poll_after_ms": accepted.PollAfterMs})
}
func (h *handler) getForecast(w http.ResponseWriter, r *http.Request) {
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	result, err := h.core.Forecasts.GetForecast(r.Context(), &corev1.GetForecastRequest{Context: browser(r), Scope: scopeFor(account), ForecastId: r.PathValue("forecast_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	f := result.GetForecast()
	if !validForecast(f, account, r.PathValue("forecast_id")) {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, forecastDTO(f))
}
func scopeFor(a *corev1.Account) *corev1.AccountScope {
	return &corev1.AccountScope{CompanyId: a.CompanyId, AccountId: a.Id}
}
func dependencyError(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
}
