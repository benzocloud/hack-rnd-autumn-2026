package forecasting

import "errors"

var (
	ErrModelInput       = errors.New("invalid model input")
	ErrModelContext     = errors.New("model context or calendar unsupported")
	ErrModelUnavailable = errors.New("model service unavailable")
	ErrModelBusy        = errors.New("model capacity exhausted")
	ErrModelResponse    = errors.New("invalid model response")
)
