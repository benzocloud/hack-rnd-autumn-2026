package log

import (
	"errors"
	"io"
	"log/slog"
	"strings"
)

// New writes JSON or text logs to w at level. The caller owns the writer.
func New(w io.Writer, level slog.Level, format string) (*slog.Logger, error) {
	if w == nil {
		return nil, errors.New("log writer is required")
	}
	options := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return slog.New(slog.NewJSONHandler(w, options)), nil
	case "text":
		return slog.New(slog.NewTextHandler(w, options)), nil
	default:
		return nil, errors.New("log format must be json or text")
	}
}
