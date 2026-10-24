package account

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (s *service) checkContext(ctx context.Context, users userStore, contexts contextStore, input WriteContext, write, session bool) (domain.SecurityContext, domain.User, error) {
	hash, valid := tokenHash(input.CookieToken)
	if !valid {
		return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
	}
	peek, err := contexts.FindContext(ctx, hash, false)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
	}
	if err != nil {
		return domain.SecurityContext{}, domain.User{}, err
	}
	var user domain.User
	if peek.Kind == "session" {
		user, err = users.FindUserByID(ctx, peek.UserID, true)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
		}
		if err != nil {
			return domain.SecurityContext{}, domain.User{}, err
		}
	}
	current, err := contexts.FindContext(ctx, hash, true)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
	}
	if err != nil {
		return domain.SecurityContext{}, domain.User{}, err
	}
	now := s.now().UTC()
	if current.ID != peek.ID || current.Kind != peek.Kind || current.UserID != peek.UserID || current.CSRFKeyID != "primary" || current.RevokedAt != nil || !now.Before(current.ExpiresAt) {
		return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
	}
	switch current.Kind {
	case "pre_auth":
		if session {
			return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
		}
	case "session":
		if user.State != "active" || current.CredentialVersion != user.CredentialVersion || !now.Before(current.LastSeenAt.Add(30*time.Minute)) || (s.demo && !s.demoEmails[user.Email]) {
			return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
		}
	default:
		return domain.SecurityContext{}, domain.User{}, ErrAuthRequired
	}
	if write && !hmac.Equal([]byte(input.CSRFToken), []byte(s.csrfToken(current))) {
		return domain.SecurityContext{}, domain.User{}, ErrCSRFInvalid
	}
	if current.Kind == "session" {
		if err := contexts.TouchSession(ctx, current.ID, now); err != nil {
			return domain.SecurityContext{}, domain.User{}, err
		}
	}
	return current, user, nil
}
func (s *service) newContext(user domain.User) (domain.SecurityContext, string, error) {
	token, hash, err := opaqueToken()
	if err != nil {
		return domain.SecurityContext{}, "", ErrUnavailable
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return domain.SecurityContext{}, "", ErrUnavailable
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return domain.SecurityContext{}, "", ErrUnavailable
	}
	now := s.now().UTC()
	c := domain.SecurityContext{ID: id, Kind: "pre_auth", TokenHash: hash, CSRFNonce: nonce, CSRFKeyID: "primary", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if user.ID != uuid.Nil {
		c.Kind = "session"
		c.UserID = user.ID
		c.CredentialVersion = user.CredentialVersion
		c.LastSeenAt = now
		c.ExpiresAt = now.Add(24 * time.Hour)
	}
	return c, token, nil
}
func (s *service) GetSecurityContext(ctx context.Context, cookie string) (SecurityContext, error) {
	var result SecurityContext
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore) error {
		current, _, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: cookie}, false, false)
		token := cookie
		if errors.Is(err, ErrAuthRequired) {
			current, token, err = s.newContext(domain.User{})
			if err == nil {
				err = contexts.InsertContext(ctx, current)
			}
		}
		if err != nil {
			return err
		}
		result = SecurityContext{CookieToken: token, CSRFToken: s.csrfToken(current), ExpiresAt: current.ExpiresAt}
		return nil
	})
	if err != nil {
		return SecurityContext{}, err
	}
	return result, nil
}
func (s *service) GetCurrentAccount(ctx context.Context, cookie string) (User, error) {
	var result User
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: cookie}, false, true)
		if err != nil {
			return err
		}
		result = s.publicUser(user)
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return result, nil
}
func (s *service) Logout(ctx context.Context, input WriteContext) error {
	return s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore) error {
		current, _, err := s.checkContext(ctx, users, contexts, input, true, false)
		if err != nil {
			return err
		}
		return contexts.RevokeContext(ctx, current.ID, s.now().UTC())
	})
}
