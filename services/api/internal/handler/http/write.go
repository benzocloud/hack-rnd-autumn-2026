package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

const challengeMessage = "Если подтверждение требуется, письмо с кодом будет отправлено."
const genericMessage = "Если адрес зарегистрирован, инструкции будут отправлены."

func parse(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := decode(w, r, into); err != nil {
		var typeError *json.UnmarshalTypeError
		if errors.As(err, &typeError) {
			invalid(w, r)
		} else {
			writeError(w, r, 400, "INVALID_REQUEST", nil)
		}
		return false
	}
	return true
}
func invalid(w http.ResponseWriter, r *http.Request) { writeError(w, r, 422, "VALIDATION_ERROR", nil) }

func respondChallenge(w http.ResponseWriter, r *http.Request, c *authv1.EmailVerificationChallenge) {
	if c == nil || !uuid4.MatchString(c.GetChallengeId()) || !validTime(c.GetExpiresAt()) {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	writeJSON(w, 202, challenge{challengeMessage, c.GetChallengeId(), c.GetExpiresAt().AsTime().UTC().Format(time.RFC3339Nano)})
}
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var d registerRequest
	if !parse(w, r, &d) {
		return
	}
	if !emailValid(d.Email) || !passwordValid(d.Password) || !nameValid(d.DisplayName) {
		invalid(w, r)
		return
	}
	if d.Consent == nil || !d.Consent.Accepted {
		writeError(w, r, 422, "CONSENT_REQUIRED", nil)
		return
	}
	if utf8.RuneCountInString(d.Consent.Version) < 1 || utf8.RuneCountInString(d.Consent.Version) > 64 || !sha256hex.MatchString(d.Consent.Hash) {
		invalid(w, r)
		return
	}
	p, err := h.auth.Register(r.Context(), &authv1.RegisterRequest{Context: writeContext(r), Email: d.Email, Password: d.Password, DisplayName: d.DisplayName, PersonalDataConsent: &authv1.ConsentAcceptance{Accepted: d.Consent.Accepted, DocumentVersion: d.Consent.Version, DocumentSha256: d.Consent.Hash}})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	respondChallenge(w, r, p.GetChallenge())
}
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var d loginRequest
	if !parse(w, r, &d) {
		return
	}
	if !emailValid(d.Email) || !passwordValid(d.Password) {
		invalid(w, r)
		return
	}
	p, err := h.auth.Login(r.Context(), &authv1.LoginRequest{Context: writeContext(r), Email: d.Email, Password: d.Password})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	if !validUser(p.GetUser()) || !validSecurity(p.GetContext()) {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	c := p.GetContext()
	setCookie(w, c.GetCookieToken(), c.GetCookieExpiresAt().AsTime())
	writeJSON(w, 200, session{userDTO(p.GetUser()), c.GetCookieExpiresAt().AsTime().UTC().Format(time.RFC3339Nano)})
}
func (h *handler) verify(w http.ResponseWriter, r *http.Request) {
	var d verifyRequest
	if !parse(w, r, &d) {
		return
	}
	if !uuid4.MatchString(d.Challenge) || !code6.MatchString(d.Code) {
		invalid(w, r)
		return
	}
	_, err := h.auth.VerifyEmail(r.Context(), &authv1.VerifyEmailRequest{Context: writeContext(r), ChallengeId: d.Challenge, Code: d.Code})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	writeJSON(w, 200, acknowledgement{"Подтверждение обработано."})
}
func (h *handler) resend(w http.ResponseWriter, r *http.Request) {
	var d emailRequest
	if !parse(w, r, &d) {
		return
	}
	if !emailValid(d.Email) {
		invalid(w, r)
		return
	}
	p, err := h.auth.ResendVerification(r.Context(), &authv1.ResendVerificationRequest{Context: writeContext(r), Email: d.Email})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	respondChallenge(w, r, p.GetChallenge())
}
func (h *handler) forgot(w http.ResponseWriter, r *http.Request) {
	var d emailRequest
	if !parse(w, r, &d) {
		return
	}
	if !emailValid(d.Email) {
		invalid(w, r)
		return
	}
	_, err := h.auth.ForgotPassword(r.Context(), &authv1.ForgotPasswordRequest{Context: writeContext(r), Email: d.Email})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	writeJSON(w, 202, acknowledgement{genericMessage})
}
func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	var d resetRequest
	if !parse(w, r, &d) {
		return
	}
	n := utf8.RuneCountInString(d.Token)
	if n < 16 || n > 512 || !passwordValid(d.Password) {
		invalid(w, r)
		return
	}
	_, err := h.auth.ResetPassword(r.Context(), &authv1.ResetPasswordRequest{Context: writeContext(r), Token: d.Token, NewPassword: d.Password})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	clearCookie(w)
	writeJSON(w, 200, acknowledgement{"Пароль изменён."})
}
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength != 0 {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	_, err := h.auth.Logout(r.Context(), &authv1.LogoutRequest{Context: writeContext(r)})
	if err != nil {
		rpcError(w, r, err)
		return
	}
	clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
