package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

func rpcError(err error) error {
	code := codes.Internal
	reason := pb.AuthErrorReason_AUTH_ERROR_REASON_INTERNAL_ERROR
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	case errors.Is(err, account.ErrInvalidCursor):
		code = codes.InvalidArgument
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CURSOR
	case errors.Is(err, account.ErrValidation):
		code = codes.InvalidArgument
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_VALIDATION_ERROR
	case errors.Is(err, account.ErrAuthRequired):
		code = codes.Unauthenticated
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED
	case errors.Is(err, account.ErrInvalidCredentials):
		code = codes.Unauthenticated
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS
	case errors.Is(err, account.ErrCSRFInvalid):
		code = codes.PermissionDenied
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID
	case errors.Is(err, account.ErrConsentRequired):
		code = codes.InvalidArgument
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_REQUIRED
	case errors.Is(err, account.ErrConsentChanged):
		code = codes.Aborted
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_DOCUMENT_CHANGED
	case errors.Is(err, account.ErrEmailUnverified):
		code = codes.PermissionDenied
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_EMAIL_UNVERIFIED
	case errors.Is(err, account.ErrCodeInvalid):
		code = codes.InvalidArgument
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_CODE_INVALID_OR_EXPIRED
	case errors.Is(err, account.ErrForbidden):
		code = codes.PermissionDenied
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_FORBIDDEN
	case errors.Is(err, account.ErrNotFound):
		code = codes.NotFound
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_NOT_FOUND
	case errors.Is(err, account.ErrUnavailable):
		code = codes.Unavailable
		reason = pb.AuthErrorReason_AUTH_ERROR_REASON_DEPENDENCY_UNAVAILABLE
	}
	result, detailErr := status.New(code, "auth request failed").WithDetails(&pb.AuthErrorDetail{Reason: reason})
	if detailErr != nil {
		return status.Error(codes.Internal, "auth request failed")
	}
	return result.Err()
}
