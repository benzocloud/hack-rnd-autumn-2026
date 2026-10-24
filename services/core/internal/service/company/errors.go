package company

import (
	"context"
	"errors"
)

var (
	ErrInvalid      = errors.New("invalid company command")
	ErrUnauthorized = errors.New("company bootstrap authorization mismatch")
	ErrConflict     = errors.New("company bootstrap conflict")
	ErrUnavailable  = errors.New("company storage or authorization unavailable")
	ErrNotFound     = errors.New("company resource not found")
)

// Auth transport details may contain private diagnostics. Keep causes inspectable
// while returning only a safe classification, as at existing storage boundaries.
type authorizationError struct{ classification, cause error }

func (e *authorizationError) Error() string   { return e.classification.Error() }
func (e *authorizationError) Unwrap() []error { return []error{e.classification, e.cause} }
func mapAuthorizationError(err error) error {
	classification := ErrUnavailable
	for _, expected := range []error{context.Canceled, context.DeadlineExceeded, ErrUnauthorized} {
		if errors.Is(err, expected) {
			classification = expected
			break
		}
	}
	return &authorizationError{classification: classification, cause: err}
}
