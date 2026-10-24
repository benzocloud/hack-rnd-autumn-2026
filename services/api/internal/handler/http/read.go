package httpapi

import (
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

func validTime(t *timestamppb.Timestamp) bool {
	return t != nil && t.CheckValid() == nil && t.AsTime().After(time.Now())
}
func validSecurity(c *authv1.SecurityContext) bool {
	return c != nil && opaque.MatchString(c.GetCookieToken()) && len(c.GetCsrfToken()) >= 16 && len(c.GetCsrfToken()) <= 512 && validTime(c.GetCookieExpiresAt())
}
func validUser(u *authv1.UserProfile) bool {
	return u != nil && len(u.GetId()) >= 1 && len(u.GetId()) <= 128 && emailValid(u.GetEmail()) && nameValid(u.GetDisplayName())
}

func (h *handler) consent(w http.ResponseWriter, r *http.Request) {
	p, err := h.auth.GetPersonalDataConsent(r.Context(), &authv1.GetPersonalDataConsentRequest{})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	d := p.GetDocument()
	if d == nil || d.GetText() == "" || len(d.GetDocumentVersion()) < 1 || len(d.GetDocumentVersion()) > 64 || !sha256hex.MatchString(d.GetDocumentSha256()) {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	writeJSON(w, 200, document{d.GetText(), d.GetDocumentVersion(), d.GetDocumentSha256()})
}
func (h *handler) csrf(w http.ResponseWriter, r *http.Request) {
	p, err := h.auth.GetSecurityContext(r.Context(), &authv1.GetSecurityContextRequest{Context: browser(r), Client: clientContext(r)})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	c := p.GetContext()
	if !validSecurity(c) {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	setCookie(w, c.GetCookieToken(), c.GetCookieExpiresAt().AsTime())
	writeJSON(w, 200, csrf{c.GetCsrfToken(), c.GetCookieExpiresAt().AsTime().UTC().Format(time.RFC3339Nano)})
}
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	p, err := h.auth.GetCurrentAccount(r.Context(), &authv1.GetCurrentAccountRequest{Context: browser(r)})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	if !validUser(p.GetUser()) {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	writeJSON(w, 200, userDTO(p.GetUser()))
}
