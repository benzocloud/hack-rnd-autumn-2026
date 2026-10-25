package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// These small flat mutation DTOs require exact keys and explicit presence;
// encoding/json alone would accept case variants, null scalars and duplicates.
func ledgerBody(w http.ResponseWriter, r *http.Request, required ...string) (map[string]json.RawMessage, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, errors.New("JSON content type required")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil || !utf8.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return nil, errors.New("invalid JSON body")
	}
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != len(required) {
		return nil, errors.New("invalid JSON fields")
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return nil, errors.New("missing JSON field")
		}
	}
	return fields, nil
}

func ledgerVersion(raw json.RawMessage) (int64, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || len(value) == 0 {
		return 0, false
	}
	version, err := strconv.ParseInt(value, 10, 64)
	return version, err == nil && version > 0 && strconv.FormatInt(version, 10) == value
}
