package httpapi

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type createCompanyRequest struct {
	Name     string  `json:"name"`
	INN      *string `json:"inn"`
	Currency string  `json:"currency"`
	Timezone string  `json:"timezone"`
}

func (h *handler) createCompany(w http.ResponseWriter, r *http.Request) {
	var body createCompanyRequest
	keys := r.Header.Values("Idempotency-Key")
	if err := decode(w, r, &body); err != nil || len(keys) != 1 || !validIdempotencyKey(keys[0]) || !validCreateCompanyRequest(body) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	prepared, err := h.auth.BeginCompanyCreation(r.Context(), &authv1.BeginCompanyCreationRequest{Context: writeContext(r), IdempotencyKey: keys[0], Company: &authv1.CompanyCreateSpec{Name: body.Name, Inn: body.INN, Currency: body.Currency, Timezone: body.Timezone}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	command := prepared.GetCommand()
	if !validCreationCommand(command) {
		dependencyError(w, r)
		return
	}
	if command.State == authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED {
		writeError(w, r, 403, "FORBIDDEN", nil)
		return
	}
	// Forward Auth's saved spec unchanged. The browser payload is not authoritative.
	_, err = h.core.Companies.CreateCompanyBootstrap(r.Context(), &corev1.CreateCompanyBootstrapRequest{CommandId: command.Id, CompanyId: command.CompanyId, CompanySpec: command.Company, RequestSha256: command.RequestSha256})
	if err != nil {
		companyCreationCoreError(w, r, err)
		return
	}
	// A bootstrap acknowledgement does not grant membership. Auth reads the receipt.
	completed, err := h.auth.CompleteCompanyCreation(r.Context(), &authv1.CompleteCompanyCreationRequest{Context: writeContext(r), CommandId: command.Id})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	result := completed.GetCommand()
	if !validCreationCommand(result) || result.Id != command.Id || result.CompanyId != command.CompanyId || result.InitiatorId != command.InitiatorId || result.RequestSha256 != command.RequestSha256 || !proto.Equal(result.Company, command.Company) {
		dependencyError(w, r)
		return
	}
	if result.State == authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED {
		writeError(w, r, 403, "FORBIDDEN", nil)
		return
	}
	if result.State != authv1.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED {
		writeError(w, r, 503, "COMPANY_CREATION_IN_PROGRESS", nil)
		return
	}
	read, err := h.core.Companies.ReadCompany(r.Context(), &corev1.ReadCompanyRequest{Context: browser(r), CompanyId: command.CompanyId})
	if err != nil {
		companyCreationCoreError(w, r, err)
		return
	}
	value, err := companyDTO(read, command.CompanyId)
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 201, value)
}

// Creation uses the durable Auth command's 503/retry semantics. Other Core
// endpoints retain their existing deadline mapping.
func companyCreationCoreError(w http.ResponseWriter, r *http.Request, err error) {
	if status.Code(err) == codes.DeadlineExceeded {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	coreRPCError(w, r, err)
}

func validCreateCompanyRequest(body createCompanyRequest) bool {
	return nonempty(body.Name, 160) && strings.TrimSpace(body.Name) != "" && !strings.ContainsRune(body.Name, 0) && body.Currency == "RUB" && body.Timezone == "Europe/Moscow" && (body.INN == nil || innPattern.MatchString(*body.INN))
}

// Validate the trusted dependency projection without renormalizing its spec or
// duplicating Auth's fingerprint algorithm in the adapter.
func validCreationCommand(c *authv1.CompanyCreationCommand) bool {
	if c == nil || !uuid4.MatchString(c.Id) || !uuid7.MatchString(c.CompanyId) || !uuid4.MatchString(c.InitiatorId) || !sha256hex.MatchString(c.RequestSha256) || !timestamp(c.CreatedAt) || c.Company == nil {
		return false
	}
	spec := c.Company
	if !nonempty(spec.Name, 160) || spec.Name != strings.TrimSpace(spec.Name) || strings.ContainsRune(spec.Name, 0) || spec.Currency != "RUB" || spec.Timezone != "Europe/Moscow" || spec.Inn != nil && !innPattern.MatchString(*spec.Inn) {
		return false
	}
	return c.State == authv1.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED || c.State == authv1.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED || c.State == authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
}
