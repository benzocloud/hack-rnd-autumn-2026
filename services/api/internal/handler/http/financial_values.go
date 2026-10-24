package httpapi

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type object map[string]any

var resourcePattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var innPattern = regexp.MustCompile(`^(?:[0-9]{10}|[0-9]{12})$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
var machineCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func resourceID(s string) bool {
	return resourcePattern.MatchString(s) && s != "00000000-0000-0000-0000-000000000000"
}
func optionalID(s *string) bool     { return s == nil || resourceID(*s) }
func bounded(s string, n int) bool  { return utf8.ValidString(s) && utf8.RuneCountInString(s) <= n }
func nonempty(s string, n int) bool { return len(s) > 0 && bounded(s, n) }
func businessDate(s string) bool {
	d, e := time.Parse(time.DateOnly, s)
	return e == nil && len(s) == 10 && d.Year() > 0 && d.Format(time.DateOnly) == s
}
func timestamp(t *timestamppb.Timestamp) bool { return t != nil && t.CheckValid() == nil }
func utc(t *timestamppb.Timestamp) string     { return t.AsTime().UTC().Format(time.RFC3339Nano) }
func optionalText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func textPointer(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
func stringsDTO(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func validCursor(s string) bool {
	if len(s) > 1024 {
		return false
	}
	for _, c := range []byte(s) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func validIdempotencyKey(s string) bool              { return keyPattern.MatchString(s) }
func parseQuery(r *http.Request) (url.Values, error) { return url.ParseQuery(r.URL.RawQuery) }
func roleName(r authv1.CompanyRole) string {
	switch r {
	case authv1.CompanyRole_COMPANY_ROLE_UNSPECIFIED:
		return ""
	case authv1.CompanyRole_COMPANY_ROLE_OWNER:
		return "owner"
	case authv1.CompanyRole_COMPANY_ROLE_FINANCIER:
		return "financier"
	}
	return ""
}
func moneyDTO(m *corev1.Money) any {
	if m == nil {
		return nil
	}
	return object{"amount_minor": strconv.FormatInt(m.AmountMinor, 10), "currency": m.Currency}
}
func validMoney(m *corev1.Money) bool       { return m != nil && m.Currency == "RUB" }
func positiveMoney(m *corev1.Money) bool    { return validMoney(m) && m.AmountMinor > 0 }
func nonnegativeMoney(m *corev1.Money) bool { return validMoney(m) && m.AmountMinor >= 0 }
func companyDTO(r *corev1.ReadCompanyResponse, id string) (object, error) {
	c := r.GetCompany()
	if c == nil || c.Id != id || !resourceID(c.Id) || !resourceID(c.DefaultAccountId) || !nonempty(c.Name, 160) || c.Currency != "RUB" || c.Timezone != "Europe/Moscow" || c.Version < 1 || !timestamp(c.CreatedAt) || roleName(r.Role) == "" || c.Inn != nil && !innPattern.MatchString(*c.Inn) {
		return nil, errProjection
	}
	return object{"id": c.Id, "name": c.Name, "inn": textPointer(c.Inn), "currency": c.Currency, "timezone": c.Timezone, "membership": object{"role": roleName(r.Role)}, "default_account_id": c.DefaultAccountId, "version": strconv.FormatInt(c.Version, 10), "created_at": utc(c.CreatedAt)}, nil
}
func validAccount(a *corev1.Account, company string) bool {
	if a == nil || a.CompanyId != company || !resourceID(a.Id) || !resourceID(a.CompanyId) || a.Currency != "RUB" || a.DataVersion < 1 {
		return false
	}
	if b := a.Balance; b != nil && (!validMoney(b.Amount) || !businessDate(b.AsOf) || (b.Source != corev1.BalanceSource_BALANCE_SOURCE_MANUAL && b.Source != corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD)) {
		return false
	}
	if s := a.TransactionSource; s != nil {
		switch s.Mode {
		case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_UNSPECIFIED:
			return false
		case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL:
			if s.SourceSystem != "manual_v1" || s.SourceCompanyId != nil {
				return false
			}
		case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED:
			if !nonempty(s.SourceSystem, 128) || s.SourceCompanyId == nil || !nonempty(*s.SourceCompanyId, 128) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func accountDTO(a *corev1.Account) object {
	result := object{"id": a.Id, "company_id": a.CompanyId, "name": a.Name, "currency": a.Currency, "data_version": strconv.FormatInt(a.DataVersion, 10), "balance": nil, "balance_as_of": nil, "balance_source": nil, "transaction_source": nil}
	if b := a.Balance; b != nil {
		source := "manual"
		if b.Source == corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD {
			source = "statement_eod"
		}
		result["balance"], result["balance_as_of"], result["balance_source"] = moneyDTO(b.Amount), b.AsOf, source
	}
	if s := a.TransactionSource; s != nil {
		mode := "manual"
		if s.Mode == corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED {
			mode = "external"
		}
		result["transaction_source"] = object{"mode": mode, "source_system": s.SourceSystem, "source_company_id": textPointer(s.SourceCompanyId)}
	}
	return result
}
func statusName(s corev1.ForecastStatus) string {
	switch s {
	case corev1.ForecastStatus_FORECAST_STATUS_UNSPECIFIED:
		return ""
	case corev1.ForecastStatus_FORECAST_STATUS_QUEUED:
		return "queued"
	case corev1.ForecastStatus_FORECAST_STATUS_RUNNING:
		return "running"
	case corev1.ForecastStatus_FORECAST_STATUS_SUCCEEDED:
		return "succeeded"
	case corev1.ForecastStatus_FORECAST_STATUS_FAILED:
		return "failed"
	}
	return ""
}
func dateSequence(cutoff, start, end string, horizon uint32) bool {
	if !businessDate(cutoff) || !businessDate(start) || !businessDate(end) || horizon < 30 || horizon > 90 {
		return false
	}
	day, _ := time.Parse(time.DateOnly, cutoff)
	return day.AddDate(0, 0, 1).Format(time.DateOnly) == start && day.AddDate(0, 0, int(horizon)).Format(time.DateOnly) == end
}
func validSummary(f *corev1.ForecastSummary) bool {
	if f == nil || !resourceID(f.Id) || statusName(f.Status) == "" || !timestamp(f.CreatedAt) || !optionalID(f.ScenarioId) || !businessDate(f.CutoffDate) || !businessDate(f.StartDate) || !businessDate(f.EndDate) {
		return false
	}
	cutoff, _ := time.Parse(time.DateOnly, f.CutoffDate)
	end, _ := time.Parse(time.DateOnly, f.EndDate)
	n := int(end.Sub(cutoff).Hours() / 24)
	return n >= 30 && n <= 90 && dateSequence(f.CutoffDate, f.StartDate, f.EndDate, uint32(n))
}
func summaryDTO(f *corev1.ForecastSummary) object {
	return object{"id": f.Id, "status": statusName(f.Status), "cutoff_date": f.CutoffDate, "start_date": f.StartDate, "end_date": f.EndDate, "created_at": utc(f.CreatedAt), "is_stale": f.IsStale, "scenario_id": textPointer(f.ScenarioId)}
}
func contains(value string, allowed ...string) bool {
	for _, s := range allowed {
		if s == value {
			return true
		}
	}
	return false
}
func optionalOpaque(v *string) bool { return v == nil || nonempty(*v, 128) }
func directionName(d corev1.Direction) string {
	return strings.ToLower(strings.TrimPrefix(d.String(), "DIRECTION_"))
}

func issueSeverity(s corev1.IssueSeverity) string {
	switch s {
	case corev1.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED:
		return ""
	case corev1.IssueSeverity_ISSUE_SEVERITY_WARNING:
		return "warning"
	case corev1.IssueSeverity_ISSUE_SEVERITY_BLOCKING:
		return "blocking"
	}
	return ""
}
