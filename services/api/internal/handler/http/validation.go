package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var uuid4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var code6 = regexp.MustCompile(`^[0-9]{6}$`)
var sha256hex = regexp.MustCompile(`^[a-f0-9]{64}$`)
var opaque = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func emailValid(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 254 {
		return false
	}
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s && strings.Contains(s, "@")
}
func passwordValid(s string) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= 12 && n <= 128 && len(s) <= 512
}
func nameValid(s string) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= 1 && n <= 120
}

func decode(w http.ResponseWriter, r *http.Request, into any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("JSON content type required")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil || !utf8.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return errors.New("invalid JSON body")
	}
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return err
	}
	if err := exactFields(data, into); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("extra JSON value")
	}
	return nil
}

// encoding/json accepts case-insensitive struct matches. OpenAPI keys are exact.
func exactFields(data []byte, into any) error {
	var allowed []string
	switch into.(type) {
	case *registerRequest:
		allowed = []string{"email", "password", "display_name", "personal_data_consent"}
	case *loginRequest:
		allowed = []string{"email", "password"}
	case *emailRequest:
		allowed = []string{"email"}
	case *verifyRequest:
		allowed = []string{"challenge_id", "code"}
	case *resetRequest:
		allowed = []string{"token", "new_password"}
	default:
		return errors.New("unsupported request DTO")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if !slices.Contains(allowed, key) {
			return errors.New("unknown JSON field")
		}
		if key == "personal_data_consent" {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(value, &nested); err != nil {
				return err
			}
			for name := range nested {
				if !slices.Contains([]string{"accepted", "document_version", "document_sha256"}, name) {
					return errors.New("unknown consent field")
				}
			}
		}
	}
	return nil
}

// Reject duplicate keys before decoding into a struct; different consumers must
// not disagree about which security-sensitive field wins.
func uniqueKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for d.More() {
		if delim == '{' {
			key, keyErr := d.Token()
			if keyErr != nil {
				return keyErr
			}
			name, valid := key.(string)
			if !valid || seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
		}
		if err := uniqueKeys(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
