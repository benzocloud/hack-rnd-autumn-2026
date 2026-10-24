package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

var (
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrValidation         = errors.New("auth validation failed")
	ErrAuthRequired       = errors.New("auth required")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrCSRFInvalid        = errors.New("invalid csrf")
	ErrConsentRequired    = errors.New("consent required")
	ErrConsentChanged     = errors.New("consent document changed")
	ErrEmailUnverified    = errors.New("email unverified")
	ErrCodeInvalid        = errors.New("code invalid or expired")
	ErrForbidden          = errors.New("forbidden")
	ErrNotFound           = domain.ErrNotFound
	ErrUnavailable        = domain.ErrUnavailable
)

type userStore interface {
	FindUserByEmail(context.Context, string) (domain.User, error)
	FindUserByID(context.Context, uuid.UUID, bool) (domain.User, error)
	InsertIdentity(context.Context, domain.User) (bool, error)
	SetEmailVerified(context.Context, uuid.UUID, string) error
	RecordVerificationFailure(context.Context, uuid.UUID, time.Time) error
}
type contextStore interface {
	FindContext(context.Context, []byte, bool) (domain.SecurityContext, error)
	InsertContext(context.Context, domain.SecurityContext) error
	RevokeContext(context.Context, uuid.UUID, time.Time) error
	TouchSession(context.Context, uuid.UUID, time.Time) error
}
type challengeStore interface {
	FindChallenge(context.Context, uuid.UUID, bool) (domain.Challenge, error)
	RevokeChallenges(context.Context, uuid.UUID, time.Time) error
	InsertChallenge(context.Context, domain.Challenge) error
	SaveChallenge(context.Context, domain.Challenge) error
}
type consentStore interface {
	InsertEvidence(context.Context, domain.ConsentEvidence) error
	GetActiveDocument(context.Context, bool) (domain.ConsentDocument, error)
}
type membershipStore interface {
	FindMembership(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, error)
	ListMemberships(context.Context, uuid.UUID, uuid.UUID, *time.Time, int) ([]domain.Membership, error)
}

// Work binds consumer-owned ports to one transaction in the composing application.
type Work func(context.Context, userStore, contextStore, challengeStore, consentStore, membershipStore) error

type Options struct {
	CodeKey, CSRFKey []byte
	Demo             bool
	DemoEmails       []string
	Consent          Document
	Clock            func() time.Time
}
type Document = domain.ConsentDocument
type WriteContext struct{ CookieToken, CSRFToken string }
type User struct {
	ID                 uuid.UUID
	Email, DisplayName string
	EmailVerified      bool
}
type SecurityContext struct {
	CookieToken, CSRFToken string
	ExpiresAt              time.Time
}
type Challenge struct {
	ID        uuid.UUID
	ExpiresAt time.Time
}
type RegisterInput struct {
	Context                       WriteContext
	Email, Password, DisplayName  string
	ConsentAccepted               bool
	ConsentVersion, ConsentSHA256 string
}
type LoginInput struct {
	Context         WriteContext
	Email, Password string
}
type LoginResult struct {
	User    User
	Context SecurityContext
}
type VerifyEmailInput struct {
	Context     WriteContext
	ChallengeID uuid.UUID
	Code        string
}
type AuthorizeCompanyInput struct {
	CookieToken, CSRFToken string
	CompanyID              uuid.UUID
	Permission             string
	Write                  bool
}
type Authorization struct {
	ActorID uuid.UUID
	Role    string
}
type Membership struct {
	CompanyID uuid.UUID
	Role      string
	CreatedAt time.Time
}
type MembershipPage struct {
	Items      []Membership
	NextCursor string
}

type service struct {
	withinTx         func(context.Context, Work) error
	codeKey, csrfKey []byte
	demo             bool
	demoEmails       map[string]bool
	consent          Document
	now              func() time.Time
	dummyHash        string
	hashSlots        chan struct{}
}

func New(withinTx func(context.Context, Work) error, options Options) (*service, error) {
	if err := validateOptions(withinTx, options); err != nil {
		return nil, err
	}
	now := options.Clock
	if now == nil {
		now = time.Now
	}
	emails := make(map[string]bool, len(options.DemoEmails))
	for _, email := range options.DemoEmails {
		normalized, err := normalizeEmail(email)
		if err != nil || !strings.HasSuffix(normalized, ".test") {
			return nil, ErrValidation
		}
		emails[normalized] = true
	}
	dummy, err := hashPassword("not-a-real-user-password")
	if err != nil {
		return nil, ErrUnavailable
	}
	return &service{withinTx: withinTx, codeKey: append([]byte(nil), options.CodeKey...), csrfKey: append([]byte(nil), options.CSRFKey...), demo: options.Demo, demoEmails: emails, consent: options.Consent, now: now, dummyHash: dummy, hashSlots: make(chan struct{}, 2)}, nil
}

func validateOptions(work func(context.Context, Work) error, o Options) error {
	sum := sha256.Sum256([]byte(o.Consent.Text))
	if work == nil || len(o.CodeKey) != 32 || len(o.CSRFKey) != 32 || o.Consent.Text == "" || o.Consent.Version == "" || hex.EncodeToString(sum[:]) != o.Consent.SHA256 || (o.Demo && len(o.DemoEmails) == 0) {
		return ErrValidation
	}
	return nil
}
func (s *service) GetPersonalDataConsent(ctx context.Context) (Document, error) {
	var result Document
	err := s.withinTx(ctx, func(ctx context.Context, _ userStore, _ contextStore, _ challengeStore, consents consentStore, _ membershipStore) error {
		active, err := consents.GetActiveDocument(ctx, false)
		if err != nil {
			return err
		}
		if !validDocument(active) {
			return ErrUnavailable
		}
		result = active
		return nil
	})
	if err != nil {
		return Document{}, err
	}
	return result, nil
}
func validDocument(d Document) bool {
	sum := sha256.Sum256([]byte(d.Text))
	return d.Text != "" && d.Version != "" && hex.EncodeToString(sum[:]) == d.SHA256
}
func (s *service) EmailVerificationMode() string {
	if s.demo {
		return "demo"
	}
	return "strict"
}
func (s *service) publicUser(u domain.User) User {
	return User{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, EmailVerified: s.emailVerified(u)}
}
func (s *service) emailVerified(u domain.User) bool {
	return u.EmailVerified && (u.EmailVerificationMethod != "demo" || (s.demo && s.demoEmails[u.Email]))
}
