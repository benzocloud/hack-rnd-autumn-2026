package account

import (
	"context"
	"crypto/hmac"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (s *service) VerifyEmail(ctx context.Context, input VerifyEmailInput) error {
	if input.ChallengeID == uuid.Nil || !validCode(input.Code) {
		return ErrValidation
	}
	// Validate the caller before looking up even an inert/unknown challenge. This
	// avoids turning differing CSRF errors into an account-enumeration oracle.
	if err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		_, _, err := s.checkContext(ctx, users, contexts, input.Context, true, false)
		return err
	}); err != nil {
		return err
	}
	var rejection error
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, challenges challengeStore, _ consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		peek, err := challenges.FindChallenge(ctx, input.ChallengeID, false)
		if errors.Is(err, domain.ErrNotFound) {
			rejection = ErrCodeInvalid
			return nil
		}
		if err != nil {
			return err
		}
		hash, valid := tokenHash(input.Context.CookieToken)
		if !valid {
			return ErrAuthRequired
		}
		actor, err := contexts.FindContext(ctx, hash, false)
		if errors.Is(err, domain.ErrNotFound) {
			return ErrAuthRequired
		}
		if err != nil {
			return err
		}
		if actor.Kind == "session" && actor.UserID != peek.UserID {
			rejection = ErrCodeInvalid
			return nil
		}
		user, err := users.FindUserByID(ctx, peek.UserID, true)
		if err != nil {
			return err
		}
		if _, _, err := s.checkContext(ctx, users, contexts, input.Context, true, false); err != nil {
			return err
		}
		challenge, err := challenges.FindChallenge(ctx, input.ChallengeID, true)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if challenge.UserID != user.ID || challenge.Purpose != "email_verification" || challenge.CodeKeyID != "primary" || challenge.RevokedAt != nil || challenge.FailedAttempts >= 5 || !now.Before(challenge.ExpiresAt) || user.State != "active" {
			rejection = ErrCodeInvalid
			return nil
		}
		if user.VerificationWindowStartedAt != nil && now.Before(user.VerificationWindowStartedAt.Add(15*time.Minute)) && user.VerificationFailures >= 5 {
			rejection = ErrCodeInvalid
			return nil
		}
		matches := hmac.Equal(challenge.CodeHMAC, codeMAC(s.codeKey, challenge.ID, user.ID, input.Code))
		if s.demo && s.demoEmails[user.Email] {
			matches = true
		}
		if !matches {
			challenge.FailedAttempts++
			if err := users.RecordVerificationFailure(ctx, user.ID, now); err != nil {
				return err
			}
			if err := challenges.SaveChallenge(ctx, challenge); err != nil {
				return err
			}
			rejection = ErrCodeInvalid
			return nil
		}
		if challenge.ConsumedAt != nil {
			if user.EmailVerificationMethod == "demo" && !s.demo {
				rejection = ErrCodeInvalid
			}
			return nil
		}
		challenge.ConsumedAt = &now
		if err := challenges.SaveChallenge(ctx, challenge); err != nil {
			return err
		}
		method := "strict"
		if s.demo && s.demoEmails[user.Email] {
			method = "demo"
		}
		return users.SetEmailVerified(ctx, user.ID, method)
	})
	if err != nil {
		return err
	}
	return rejection
}
