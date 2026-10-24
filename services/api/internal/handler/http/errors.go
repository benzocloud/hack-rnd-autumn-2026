package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

// Only known reason/status combinations cross the public boundary. Never copy
// upstream status text or field descriptions, which may contain request values.
func rpcError(w http.ResponseWriter, r *http.Request, err error) {
	st := status.Convert(err)
	for _, raw := range st.Details() {
		d, ok := raw.(*authv1.AuthErrorDetail)
		if !ok {
			continue
		}
		code := strings.TrimPrefix(d.GetReason().String(), "AUTH_ERROR_REASON_")
		expected, httpStatus := reasonStatus(code)
		if httpStatus == 0 || st.Code() != expected {
			break
		}
		if httpStatus == 429 {
			w.Header().Set("Retry-After", strconv.FormatUint(uint64(max(1, d.GetRetryAfterSeconds())), 10))
		}
		fields := make([]fieldError, 0, len(d.GetFieldViolations()))
		for _, f := range d.GetFieldViolations() {
			switch f.GetField() {
			case "email", "password", "display_name", "personal_data_consent", "challenge_id", "code", "token", "new_password":
				fields = append(fields, fieldError{f.GetField(), code, fieldMessage(f.GetField())})
			}
		}
		writeError(w, r, httpStatus, code, fields)
		return
	}
	if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded || st.Code() == codes.Canceled || st.Code() == codes.PermissionDenied || st.Code() == codes.Unimplemented {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	writeError(w, r, 500, "INTERNAL_ERROR", nil)
}

func reasonStatus(reason string) (codes.Code, int) {
	switch reason {
	case "INVALID_CURSOR":
		return codes.InvalidArgument, 400
	case "VALIDATION_ERROR", "CONSENT_REQUIRED", "CODE_INVALID_OR_EXPIRED", "TOKEN_INVALID_OR_EXPIRED", "INVITATION_NOT_AVAILABLE":
		return codes.InvalidArgument, 422
	case "AUTH_REQUIRED", "INVALID_CREDENTIALS":
		return codes.Unauthenticated, 401
	case "CSRF_INVALID", "EMAIL_UNVERIFIED", "FORBIDDEN":
		return codes.PermissionDenied, 403
	case "NOT_FOUND":
		return codes.NotFound, 404
	case "CONSENT_DOCUMENT_CHANGED", "VERSION_CONFLICT", "IDEMPOTENCY_CONFLICT", "INVITATION_STATE_CONFLICT", "MEMBERSHIP_REVOKED":
		return codes.Aborted, 409
	case "RATE_LIMITED":
		return codes.ResourceExhausted, 429
	case "DEPENDENCY_UNAVAILABLE", "COMPANY_CREATION_IN_PROGRESS":
		return codes.Unavailable, 503
	case "INTERNAL_ERROR":
		return codes.Internal, 500
	default:
		return codes.Unknown, 0
	}
}

func writeError(w http.ResponseWriter, r *http.Request, code int, reason string, fields []fieldError) {
	if fields == nil {
		fields = []fieldError{}
	}
	writeJSON(w, code, errorResponse{errorDetail{reason, errorMessage(reason), fields, r.Header.Get("X-Request-ID")}})
}
