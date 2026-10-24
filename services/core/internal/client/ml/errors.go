package ml

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

// Driver/server diagnostics remain inspectable without leaking their message.
type modelError struct{ classification, cause error }

func (e *modelError) Error() string   { return e.classification.Error() }
func (e *modelError) Unwrap() []error { return []error{e.classification, e.cause} }

func mapError(err error) error {
	classification := forecasting.ErrModelUnavailable
	switch status.Code(err) {
	case codes.InvalidArgument:
		classification = forecasting.ErrModelInput
	case codes.FailedPrecondition:
		classification = forecasting.ErrModelContext
	case codes.ResourceExhausted:
		classification = forecasting.ErrModelBusy
	case codes.Canceled:
		classification = context.Canceled
	case codes.DeadlineExceeded:
		classification = context.DeadlineExceeded
	case codes.OK, codes.Unknown, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied,
		codes.Aborted, codes.OutOfRange, codes.Unimplemented, codes.Internal, codes.Unavailable,
		codes.DataLoss, codes.Unauthenticated:
	}
	return &modelError{classification: classification, cause: err}
}
