package account

import (
	"context"
	"errors"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (s *service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil || !validPassword(input.Password) {
		return LoginResult{}, ErrInvalidCredentials
	}
	var observed domain.User
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		if _, _, err := s.checkContext(ctx, users, contexts, input.Context, true, false); err != nil {
			return err
		}
		user, err := users.FindUserByEmail(ctx, email)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		observed = user
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	encoded := observed.PasswordHash
	if encoded == "" {
		encoded = s.dummyHash
	}
	matches, err := s.comparePassword(ctx, encoded, input.Password)
	if err != nil {
		return LoginResult{}, err
	}
	if !matches || observed.PasswordHash == "" || (s.demo && !s.demoEmails[email]) {
		return LoginResult{}, ErrInvalidCredentials
	}
	var result LoginResult
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		h, ok := tokenHash(input.Context.CookieToken)
		if !ok {
			return ErrAuthRequired
		}
		actor, err := contexts.FindContext(ctx, h, false)
		if errors.Is(err, domain.ErrNotFound) {
			return ErrAuthRequired
		}
		if err != nil {
			return err
		}
		if actor.Kind == "session" && actor.UserID != observed.ID {
			return ErrAuthRequired
		}
		user, err := users.FindUserByID(ctx, observed.ID, true)
		if err != nil {
			return err
		}
		if user.State != "active" || user.CredentialVersion != observed.CredentialVersion || user.PasswordHash != observed.PasswordHash {
			return ErrInvalidCredentials
		}
		old, _, err := s.checkContext(ctx, users, contexts, input.Context, true, false)
		if err != nil {
			return err
		}
		current, token, err := s.newContext(user)
		if err != nil {
			return err
		}
		if err := contexts.RevokeContext(ctx, old.ID, s.now().UTC()); err != nil {
			return err
		}
		if err := contexts.InsertContext(ctx, current); err != nil {
			return err
		}
		result = LoginResult{User: s.publicUser(user), Context: SecurityContext{CookieToken: token, CSRFToken: s.csrfToken(current), ExpiresAt: current.ExpiresAt}}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	return result, nil
}
