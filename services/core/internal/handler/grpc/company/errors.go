package company

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

func mapError(err error) error {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	}
	for _, raw := range status.Convert(err).Details() {
		if detail, ok := raw.(*authpb.AuthErrorDetail); ok {
			return mapAuthError(status.Code(err), detail)
		}
	}
	code, reason := codes.Internal, pb.CoreErrorReason_CORE_ERROR_REASON_INTERNAL_ERROR
	switch {
	case errors.Is(err, company.ErrInvalid), errors.Is(err, domain.ErrInvalid):
		code, reason = codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_VALIDATION_ERROR
	case errors.Is(err, company.ErrNotFound):
		code, reason = codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND
	case errors.Is(err, company.ErrConflict):
		code, reason = codes.Aborted, pb.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT
	case errors.Is(err, company.ErrUnauthorized):
		code, reason = codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_FORBIDDEN
	case errors.Is(err, company.ErrUnavailable), status.Code(err) == codes.Unavailable:
		code, reason = codes.Unavailable, pb.CoreErrorReason_CORE_ERROR_REASON_DEPENDENCY_UNAVAILABLE
	}
	return coreError(code, reason)
}

func mapAuthError(code codes.Code, detail *authpb.AuthErrorDetail) error {
	reason := detail.Reason
	switch {
	case code == codes.Unauthenticated && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED:
		return coreError(codes.Unauthenticated, pb.CoreErrorReason_CORE_ERROR_REASON_AUTH_REQUIRED)
	case code == codes.PermissionDenied && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_FORBIDDEN:
		return coreError(codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_FORBIDDEN)
	case code == codes.PermissionDenied && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID:
		return coreError(codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_CSRF_INVALID)
	case code == codes.PermissionDenied && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_EMAIL_UNVERIFIED:
		return coreError(codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_EMAIL_UNVERIFIED)
	case code == codes.NotFound && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_NOT_FOUND:
		return coreError(codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND)
	case code == codes.Aborted && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED:
		return coreError(codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_MEMBERSHIP_REVOKED)
	case code == codes.ResourceExhausted && reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_RATE_LIMITED:
		return errorWithDetail(codes.ResourceExhausted, &pb.CoreErrorDetail{Reason: pb.CoreErrorReason_CORE_ERROR_REASON_RATE_LIMITED, RetryAfterSeconds: detail.RetryAfterSeconds})
	default:
		return coreError(codes.Unavailable, pb.CoreErrorReason_CORE_ERROR_REASON_DEPENDENCY_UNAVAILABLE)
	}
}

func coreError(code codes.Code, reason pb.CoreErrorReason) error {
	return errorWithDetail(code, &pb.CoreErrorDetail{Reason: reason})
}

func errorWithDetail(code codes.Code, detail *pb.CoreErrorDetail) error {
	result, err := status.New(code, "Core request failed").WithDetails(detail)
	if err != nil {
		return status.Error(codes.Internal, "Core request failed")
	}
	return result.Err()
}
