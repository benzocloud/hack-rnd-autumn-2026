package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type createInvitationBody struct {
	Email string `json:"email"`
}

type versionBody struct {
	Version string `json:"version"`
}

func (h *handler) teamRoute(w http.ResponseWriter, r *http.Request) bool {
	if h.myInvitationsRoute(w, r) {
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/companies/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/companies/"), "/")
	if len(parts) < 2 || (parts[1] != "members" && parts[1] != "invitations") {
		return false
	}
	if !resourceID(parts[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	method := http.MethodGet
	var action func(http.ResponseWriter, *http.Request)
	switch {
	case len(parts) == 2 && parts[1] == "members":
		action = h.listMembers
	case len(parts) == 3 && parts[1] == "members":
		method, action = http.MethodDelete, h.removeMember
	case len(parts) == 2 && parts[1] == "invitations":
		action = h.listCompanyInvitations
		if r.Method == http.MethodPost {
			method, action = http.MethodPost, h.createInvitation
		}
	case len(parts) == 3 && parts[1] == "invitations":
		method, action = http.MethodDelete, h.revokeInvitation
	default:
		return false
	}
	if r.Method != method {
		if len(parts) == 2 && parts[1] == "invitations" {
			w.Header().Set("Allow", "GET, POST")
		} else {
			w.Header().Set("Allow", method)
		}
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if len(parts) == 3 && parts[1] == "members" && !resourceID(parts[2]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if len(parts) == 3 && parts[1] == "invitations" && !uuid4.MatchString(parts[2]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if (method == http.MethodPost || method == http.MethodDelete) && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("company_id", parts[0])
	if len(parts) == 3 && parts[1] == "members" {
		r.SetPathValue("user_id", parts[2])
	}
	if len(parts) == 3 && parts[1] == "invitations" {
		r.SetPathValue("invitation_id", parts[2])
	}
	action(w, r)
	return true
}

func (h *handler) myInvitationsRoute(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if path == "/api/v1/me/invitations" {
		method, action := http.MethodGet, h.listMyInvitations
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, r, 405, "INVALID_REQUEST", nil)
			return true
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		action(w, r.WithContext(ctx))
		return true
	}
	if !strings.HasPrefix(path, "/api/v1/me/invitations/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/me/invitations/"), "/")
	if len(parts) != 2 || !uuid4.MatchString(parts[0]) || (parts[1] != "accept" && parts[1] != "decline") {
		if len(parts) >= 1 && parts[0] != "" && !uuid4.MatchString(parts[0]) {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return true
		}
		return false
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512 {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("invitation_id", parts[0])
	if parts[1] == "accept" {
		h.acceptInvitation(w, r)
	} else {
		h.declineInvitation(w, r)
	}
	return true
}

func (h *handler) listMembers(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	response, err := h.auth.ListMembers(r.Context(), &authv1.ListMembersRequest{Context: browser(r), CompanyId: r.PathValue("company_id"), Page: &authv1.PageRequest{Limit: page.Limit, Cursor: page.Cursor}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	for _, m := range response.Items {
		dto, err := memberDTO(m)
		if err != nil {
			dependencyError(w, r)
			return
		}
		items = append(items, dto)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}

func (h *handler) removeMember(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	_, err := h.auth.RemoveMember(r.Context(), &authv1.RemoveMemberRequest{Context: writeContext(r), CompanyId: r.PathValue("company_id"), UserId: r.PathValue("user_id")})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listCompanyInvitations(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	response, err := h.auth.ListOutgoingInvitations(r.Context(), &authv1.ListOutgoingInvitationsRequest{Context: browser(r), CompanyId: r.PathValue("company_id"), Page: &authv1.PageRequest{Limit: page.Limit, Cursor: page.Cursor}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	writeInvitationList(w, r, response.GetItems(), response.GetPagination())
}

func (h *handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	var body createInvitationBody
	keys := r.Header.Values("Idempotency-Key")
	if err := decode(w, r, &body); err != nil || len(keys) != 1 || !validIdempotencyKey(keys[0]) || !emailValid(body.Email) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	if h.core.Companies == nil {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	companyID := r.PathValue("company_id")
	read, err := h.core.Companies.ReadCompany(r.Context(), &corev1.ReadCompanyRequest{Context: browser(r), CompanyId: companyID})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	company := read.GetCompany()
	if company == nil || company.Id != companyID || !nonempty(company.Name, 160) || roleName(read.Role) != "owner" {
		dependencyError(w, r)
		return
	}
	created, err := h.auth.CreateInvitation(r.Context(), &authv1.CreateInvitationRequest{
		Context: writeContext(r), CompanyId: companyID, Email: body.Email,
		CompanyNameSnapshot: company.Name, IdempotencyKey: keys[0],
	})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	dto, err := invitationDTO(created.GetInvitation())
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 201, dto)
}

func (h *handler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	_, err := h.auth.RevokeInvitation(r.Context(), &authv1.RevokeInvitationRequest{Context: writeContext(r), CompanyId: r.PathValue("company_id"), InvitationId: r.PathValue("invitation_id")})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listMyInvitations(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	response, err := h.auth.ListIncomingInvitations(r.Context(), &authv1.ListIncomingInvitationsRequest{Context: browser(r), Page: &authv1.PageRequest{Limit: page.Limit, Cursor: page.Cursor}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	writeInvitationList(w, r, response.GetItems(), response.GetPagination())
}

func (h *handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	keys := r.Header.Values("Idempotency-Key")
	if err := decode(w, r, &body); err != nil || len(keys) != 1 || !validIdempotencyKey(keys[0]) || !validVersion(body.Version) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	if h.core.Companies == nil {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	accepted, err := h.auth.AcceptInvitation(r.Context(), &authv1.AcceptInvitationRequest{Context: writeContext(r), InvitationId: r.PathValue("invitation_id"), Version: body.Version, IdempotencyKey: keys[0]})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	if accepted == nil || !uuid7.MatchString(accepted.CompanyId) {
		dependencyError(w, r)
		return
	}
	read, err := h.core.Companies.ReadCompany(r.Context(), &corev1.ReadCompanyRequest{Context: browser(r), CompanyId: accepted.CompanyId})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := companyDTO(read, accepted.CompanyId)
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}

func (h *handler) declineInvitation(w http.ResponseWriter, r *http.Request) {
	var body versionBody
	if err := decode(w, r, &body); err != nil || !validVersion(body.Version) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	response, err := h.auth.DeclineInvitation(r.Context(), &authv1.DeclineInvitationRequest{Context: writeContext(r), InvitationId: r.PathValue("invitation_id"), Version: body.Version})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	dto, err := invitationDTO(response.GetInvitation())
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, dto)
}

func writeInvitationList(w http.ResponseWriter, r *http.Request, items []*authv1.Invitation, page *authv1.PageResponse) {
	if page == nil || !validCursor(page.NextCursor) || len(items) > 100 {
		dependencyError(w, r)
		return
	}
	out := make([]object, 0, len(items))
	for _, item := range items {
		dto, err := invitationDTO(item)
		if err != nil {
			dependencyError(w, r)
			return
		}
		out = append(out, dto)
	}
	writeJSON(w, 200, object{"items": out, "next_cursor": optionalText(page.NextCursor)})
}

func memberDTO(m *authv1.Member) (object, error) {
	user := m.GetUser()
	if m == nil || user == nil || !resourceID(user.Id) || !emailValid(user.Email) || !nonempty(user.DisplayName, 120) || roleName(m.Role) == "" || !timestamp(m.JoinedAt) {
		return nil, errProjection
	}
	return object{"user_id": user.Id, "email": user.Email, "display_name": user.DisplayName, "role": roleName(m.Role), "joined_at": utc(m.JoinedAt)}, nil
}

func invitationDTO(in *authv1.Invitation) (object, error) {
	status := invitationStatusName(in.GetStatus())
	if in == nil || !uuid4.MatchString(in.Id) || !uuid7.MatchString(in.CompanyId) || !nonempty(in.CompanyNameSnapshot, 160) || !emailValid(in.Email) || roleName(in.Role) != "financier" || status == "" || !timestamp(in.CreatedAt) || !timestamp(in.ExpiresAt) || !validVersion(in.Version) {
		return nil, errProjection
	}
	return object{
		"id": in.Id, "company_id": in.CompanyId, "company_name": in.CompanyNameSnapshot, "email": in.Email,
		"role": "financier", "status": status, "created_at": utc(in.CreatedAt), "expires_at": utc(in.ExpiresAt), "version": in.Version,
	}, nil
}

func invitationStatusName(s authv1.InvitationStatus) string {
	switch s {
	case authv1.InvitationStatus_INVITATION_STATUS_UNSPECIFIED:
		return ""
	case authv1.InvitationStatus_INVITATION_STATUS_PENDING:
		return "pending"
	case authv1.InvitationStatus_INVITATION_STATUS_ACCEPTED:
		return "accepted"
	case authv1.InvitationStatus_INVITATION_STATUS_DECLINED:
		return "declined"
	case authv1.InvitationStatus_INVITATION_STATUS_REVOKED:
		return "revoked"
	case authv1.InvitationStatus_INVITATION_STATUS_EXPIRED:
		return "expired"
	default:
		return ""
	}
}

func validVersion(s string) bool {
	if s == "" || s[0] == '0' {
		return false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n >= 1 && strconv.FormatInt(n, 10) == s
}
