package postgres

// WithCause exposes only the classification text while keeping private diagnostics
// inspectable through errors.Is/As. Adapters must not print the unwrapped cause.
func WithCause(classification, cause error) error {
	return &classifiedError{classification: classification, cause: cause}
}

type classifiedError struct{ classification, cause error }

func (e *classifiedError) Error() string { return e.classification.Error() }

func (e *classifiedError) Unwrap() []error { return []error{e.classification, e.cause} }
