package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

var errProjection = errors.New("invalid Core response")

func coreRPCError(w http.ResponseWriter, r *http.Request, err error) {
	st := status.Convert(err)
	if st.Code() == codes.DeadlineExceeded {
		writeError(w, r, 504, "DEPENDENCY_UNAVAILABLE", nil)
		return
	}
	for _, raw := range st.Details() {
		if _, ok := raw.(*authv1.AuthErrorDetail); ok {
			rpcError(w, r, err)
			return
		}
		detail, ok := raw.(*corev1.CoreErrorDetail)
		if !ok {
			continue
		}
		reason := strings.TrimPrefix(detail.Reason.String(), "CORE_ERROR_REASON_")
		expected, httpStatus := coreReasonStatus(reason)
		if httpStatus == 0 || st.Code() != expected {
			break
		}
		if httpStatus == 429 {
			w.Header().Set("Retry-After", strconv.FormatUint(uint64(max(1, detail.RetryAfterSeconds)), 10))
		}
		writeError(w, r, httpStatus, reason, nil)
		return
	}
	dependencyError(w, r)
}
func coreReasonStatus(reason string) (codes.Code, int) {
	switch reason {
	case "VALIDATION_ERROR":
		return codes.InvalidArgument, 422
	case "INVALID_CURSOR":
		return codes.InvalidArgument, 400
	case "AUTH_REQUIRED":
		return codes.Unauthenticated, 401
	case "FORBIDDEN", "CSRF_INVALID", "EMAIL_UNVERIFIED", "MEMBERSHIP_REVOKED":
		return codes.PermissionDenied, 403
	case "NOT_FOUND":
		return codes.NotFound, 404
	case "VERSION_CONFLICT":
		return codes.Aborted, 409
	case "IDEMPOTENCY_CONFLICT":
		return codes.AlreadyExists, 409
	case "FACTUAL_SOURCE_CONFLICT", "SOURCE_BINDING_CONFLICT", "BALANCE_OBSERVATION_CONFLICT", "PAYMENT_STATE_CONFLICT", "IMPORT_STATE_CONFLICT", "IMPORT_PREVIEW_STALE", "IMPORT_WARNINGS_NOT_ACKNOWLEDGED", "FORECAST_NOT_READY", "FORECAST_NOT_COMPARABLE", "ACTUALS_NOT_AVAILABLE", "BALANCE_AS_OF_UNAVAILABLE":
		return codes.FailedPrecondition, 409
	case "INSUFFICIENT_HISTORY", "UNSUPPORTED_MODEL_CONTEXT", "SCENARIO_NOT_SUPPORTED", "SCENARIO_TARGET_CONFLICT":
		return codes.FailedPrecondition, 422
	case "RATE_LIMITED":
		return codes.ResourceExhausted, 429
	case "PAYLOAD_TOO_LARGE":
		return codes.ResourceExhausted, 413
	case "DEPENDENCY_UNAVAILABLE":
		return codes.Unavailable, 503
	case "INTERNAL_ERROR":
		return codes.Internal, 500
	default:
		return codes.Unknown, 0
	}
}
