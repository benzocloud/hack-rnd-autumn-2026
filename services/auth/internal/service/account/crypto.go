package account

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) < 3 || len(email) > 254 {
		return "", ErrValidation
	}
	for _, c := range []byte(email) {
		if c < 33 || c > 126 {
			return "", ErrValidation
		}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email {
		return "", ErrValidation
	}
	return email, nil
}
func validPassword(password string) bool {
	count := utf8.RuneCountInString(password)
	return utf8.ValidString(password) && count >= 12 && count <= 128 && len(password) <= 512
}
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return "$argon2id$v=19$m=65536,t=3,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash), nil
}
func comparePassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=1" {
		return false, ErrUnavailable
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(want) != 32 {
		return false, ErrUnavailable
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
func opaqueToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}
func tokenHash(token string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return nil, false
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:], true
}
func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n), nil
}
func validCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range []byte(code) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func codeMAC(key []byte, challengeID, userID uuid.UUID, code string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("email_verification\x00" + challengeID.String() + "\x00" + userID.String() + "\x00" + code))
	return mac.Sum(nil)
}
func (s *service) csrfToken(c domain.SecurityContext) string {
	mac := hmac.New(sha256.New, s.csrfKey)
	_, _ = mac.Write([]byte("saldo-csrf-v1\x00"))
	_, _ = mac.Write(c.ID[:])
	_, _ = mac.Write(c.CSRFNonce)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Only two memory-hard hashes run concurrently; queued calls honor cancellation.
func (s *service) hashPassword(ctx context.Context, password string) (string, error) {
	select {
	case s.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.hashSlots }()
	result, err := hashPassword(password)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", ErrUnavailable
	}
	return result, nil
}
func (s *service) comparePassword(ctx context.Context, encoded, password string) (bool, error) {
	select {
	case s.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer func() { <-s.hashSlots }()
	result, err := comparePassword(encoded, password)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return result, err
}
