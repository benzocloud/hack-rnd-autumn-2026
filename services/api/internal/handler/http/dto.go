package httpapi

import authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"

type consent struct {
	Accepted bool   `json:"accepted"`
	Version  string `json:"document_version"`
	Hash     string `json:"document_sha256"`
}
type registerRequest struct {
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	DisplayName string   `json:"display_name"`
	Consent     *consent `json:"personal_data_consent"`
}
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type emailRequest struct {
	Email string `json:"email"`
}
type verifyRequest struct {
	Challenge string `json:"challenge_id"`
	Code      string `json:"code"`
}
type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"new_password"`
}
type user struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Verified    bool   `json:"email_verified"`
}

func userDTO(p *authv1.UserProfile) user {
	return user{p.GetId(), p.GetEmail(), p.GetDisplayName(), p.GetEmailVerified()}
}

type fieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type errorDetail struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	Fields    []fieldError `json:"field_errors"`
	RequestID string       `json:"request_id"`
}
type errorResponse struct {
	Error errorDetail `json:"error"`
}
type acknowledgement struct {
	Message string `json:"message"`
}
type challenge struct {
	Message string `json:"message"`
	ID      string `json:"challenge_id"`
	Expires string `json:"expires_at"`
}
type document struct {
	Text    string `json:"text"`
	Version string `json:"document_version"`
	Hash    string `json:"document_sha256"`
}
type csrf struct {
	Token                 string `json:"csrf_token"`
	Expires               string `json:"expires_at"`
	EmailVerificationMode string `json:"email_verification_mode,omitempty"`
}
type session struct {
	User    user   `json:"user"`
	Expires string `json:"expires_at"`
}
