package account

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (s *service) Register(ctx context.Context, input RegisterInput) (Challenge, error) {
	email, err := s.validateRegistration(input)
	if err != nil {
		return Challenge{}, err
	}
	active, err := s.GetPersonalDataConsent(ctx)
	if err != nil {
		return Challenge{}, err
	}
	if input.ConsentVersion != active.Version || input.ConsentSHA256 != active.SHA256 {
		return Challenge{}, ErrConsentChanged
	}
	hash, err := s.hashPassword(ctx, input.Password)
	if err != nil {
		return Challenge{}, err
	}
	userID, err := uuid.NewRandom()
	if err != nil {
		return Challenge{}, ErrUnavailable
	}
	challenge, err := s.newChallenge(userID)
	if err != nil {
		return Challenge{}, err
	}
	evidenceID, err := uuid.NewRandom()
	if err != nil {
		return Challenge{}, ErrUnavailable
	}
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, challenges challengeStore, consents consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		active, err := consents.GetActiveDocument(ctx, true)
		if err != nil {
			return err
		}
		if !validDocument(active) {
			return ErrUnavailable
		}
		if input.ConsentVersion != active.Version || input.ConsentSHA256 != active.SHA256 {
			return ErrConsentChanged
		}
		if _, _, err := s.checkContext(ctx, users, contexts, input.Context, true, false); err != nil {
			return err
		}
		user := domain.User{ID: userID, Email: email, DisplayName: input.DisplayName, PasswordHash: hash, State: "active", CredentialVersion: 1}
		inserted, err := users.InsertIdentity(ctx, user)
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		if err := consents.InsertEvidence(ctx, domain.ConsentEvidence{ID: evidenceID, UserID: userID, Document: active, AcceptedAt: s.now().UTC()}); err != nil {
			return err
		}
		return challenges.InsertChallenge(ctx, challenge)
	})
	if err != nil {
		return Challenge{}, err
	}
	return Challenge{ID: challenge.ID, ExpiresAt: challenge.ExpiresAt}, nil
}
func (s *service) validateRegistration(input RegisterInput) (string, error) {
	if !input.ConsentAccepted {
		return "", ErrConsentRequired
	}
	if !validAcceptanceMetadata(input.ConsentVersion, input.ConsentSHA256) {
		return "", ErrValidation
	}
	email, err := normalizeEmail(input.Email)
	if err != nil || !validPassword(input.Password) || !utf8.ValidString(input.DisplayName) || utf8.RuneCountInString(input.DisplayName) < 1 || utf8.RuneCountInString(input.DisplayName) > 120 {
		return "", ErrValidation
	}
	if s.demo && !s.demoEmails[email] {
		return "", ErrForbidden
	}
	return email, nil
}
func (s *service) newChallenge(userID uuid.UUID) (domain.Challenge, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return domain.Challenge{}, ErrUnavailable
	}
	code, err := randomCode()
	if err != nil {
		return domain.Challenge{}, ErrUnavailable
	}
	now := s.now().UTC()
	return domain.Challenge{ID: id, UserID: userID, CodeHMAC: codeMAC(s.codeKey, id, userID, code), CodeKeyID: "primary", Purpose: "email_verification", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}, nil
}
func (s *service) ResendVerification(ctx context.Context, input WriteContext, rawEmail string) (Challenge, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return Challenge{}, ErrValidation
	}
	inert, err := s.newChallenge(uuid.Nil)
	if err != nil {
		return Challenge{}, err
	}
	result := Challenge{ID: inert.ID, ExpiresAt: inert.ExpiresAt}
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, challenges challengeStore, _ consentStore, _ membershipStore, _ creationStore, _ invitationStore) error {
		peek, eligible, err := s.resendCandidate(ctx, users, contexts, input, email)
		if err != nil {
			return err
		}
		if !eligible || peek.State != "active" || s.emailVerified(peek) || (s.demo && !s.demoEmails[email]) {
			return nil
		}
		challenge, err := s.newChallenge(peek.ID)
		if err != nil {
			return err
		}
		if err := challenges.RevokeChallenges(ctx, peek.ID, s.now().UTC()); err != nil {
			return err
		}
		if err := challenges.InsertChallenge(ctx, challenge); err != nil {
			return err
		}
		result = Challenge{ID: challenge.ID, ExpiresAt: challenge.ExpiresAt}
		return nil
	})
	if err != nil {
		return Challenge{}, err
	}
	return result, nil
}

func (s *service) resendCandidate(ctx context.Context, users userStore, contexts contextStore, input WriteContext, email string) (domain.User, bool, error) {
	peek, err := users.FindUserByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		_, _, contextErr := s.checkContext(ctx, users, contexts, input, true, false)
		return domain.User{}, false, contextErr
	}
	if err != nil {
		return domain.User{}, false, err
	}
	h, valid := tokenHash(input.CookieToken)
	if !valid {
		return domain.User{}, false, ErrAuthRequired
	}
	actor, err := contexts.FindContext(ctx, h, false)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, false, ErrAuthRequired
	}
	if err != nil {
		return domain.User{}, false, err
	}
	if actor.Kind == "session" && actor.UserID != peek.ID {
		_, _, contextErr := s.checkContext(ctx, users, contexts, input, true, false)
		return domain.User{}, false, contextErr
	}
	locked, err := users.FindUserByID(ctx, peek.ID, true)
	if err != nil {
		return domain.User{}, false, err
	}
	if _, _, err := s.checkContext(ctx, users, contexts, input, true, false); err != nil {
		return domain.User{}, false, err
	}
	return locked, true, nil
}

func validAcceptanceMetadata(version, hash string) bool {
	if !utf8.ValidString(version) || utf8.RuneCountInString(version) < 1 || utf8.RuneCountInString(version) > 128 || len(hash) != 64 || hash != strings.ToLower(hash) {
		return false
	}
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == 32
}
