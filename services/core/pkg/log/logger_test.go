package log_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	corelog "github.com/benzocloud/hack-rnd-autumn-2026/services/core/pkg/log"
)

func TestNewJSON(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger, err := corelog.New(&output, slog.LevelInfo, "json")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("filtered")
	logger.With("component", "core").Info("ready", "count", 2)
	var record map[string]any
	decoder := json.NewDecoder(&output)
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "ready" || record["level"] != "INFO" || record["component"] != "core" || record["count"] != float64(2) {
		t.Fatalf("unexpected JSON record: %#v", record)
	}
	var extra map[string]any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("logger emitted more than the accepted record")
	}
}

func TestNewIsolation(t *testing.T) {
	t.Parallel()
	defaultLogger := slog.Default()
	var first, second bytes.Buffer
	one, err := corelog.New(&first, slog.LevelError, "json")
	if err != nil {
		t.Fatal(err)
	}
	two, err := corelog.New(&second, slog.LevelDebug, "text")
	if err != nil {
		t.Fatal(err)
	}
	one.Info("filtered")
	two.Debug("second only")
	if first.Len() != 0 || !strings.Contains(second.String(), "second only") {
		t.Fatal("logger outputs or thresholds interfere")
	}
	if slog.Default() != defaultLogger {
		t.Fatal("constructor changed the default logger")
	}
}

func TestNewInvalid(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing writer", "unsupported format"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logger *slog.Logger
			var err error
			if name == "missing writer" {
				logger, err = corelog.New(nil, slog.LevelInfo, "json")
			} else {
				logger, err = corelog.New(&bytes.Buffer{}, slog.LevelInfo, "xml")
			}
			if err == nil || logger != nil {
				t.Fatal("invalid options must return an error and no logger")
			}
		})
	}
}
