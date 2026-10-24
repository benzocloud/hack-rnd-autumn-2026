package postgres

// Keep driver causes inspectable without printing credentials or the DSN.
type connectionError struct {
	operation string
	cause     error
}

func (e *connectionError) Error() string { return "postgres " + e.operation + " failed" }
func (e *connectionError) Unwrap() error { return e.cause }
