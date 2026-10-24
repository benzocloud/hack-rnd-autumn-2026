package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

type memoryStore struct {
	mu           sync.Mutex
	users        map[uuid.UUID]domain.User
	contexts     map[string]domain.SecurityContext
	challenges   map[uuid.UUID]domain.Challenge
	memberships  map[uuid.UUID]domain.Membership
	evidence     []domain.ConsentEvidence
	failEvidence bool
	document     Document
}

func freshStore() *memoryStore {
	return &memoryStore{users: map[uuid.UUID]domain.User{}, contexts: map[string]domain.SecurityContext{}, challenges: map[uuid.UUID]domain.Challenge{}, memberships: map[uuid.UUID]domain.Membership{}}
}
func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (m *memoryStore) work(ctx context.Context, fn Work) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	working := freshStore()
	working.users = cloneMap(m.users)
	working.contexts = cloneMap(m.contexts)
	working.challenges = cloneMap(m.challenges)
	working.memberships = cloneMap(m.memberships)
	working.evidence = append([]domain.ConsentEvidence(nil), m.evidence...)
	working.failEvidence = m.failEvidence
	working.document = m.document
	if err := fn(ctx, working, working, working, working, working); err != nil {
		return err
	}
	m.users = working.users
	m.contexts = working.contexts
	m.challenges = working.challenges
	m.memberships = working.memberships
	m.evidence = working.evidence
	m.document = working.document
	return nil
}
func (m *memoryStore) FindUserByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}
func (m *memoryStore) FindUserByID(_ context.Context, id uuid.UUID, _ bool) (domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}
func (m *memoryStore) InsertIdentity(_ context.Context, u domain.User) (bool, error) {
	for _, old := range m.users {
		if old.Email == u.Email {
			return false, nil
		}
	}
	m.users[u.ID] = u
	return true, nil
}
func (m *memoryStore) SetEmailVerified(_ context.Context, id uuid.UUID, method string) error {
	u := m.users[id]
	u.EmailVerified = true
	u.EmailVerificationMethod = method
	m.users[id] = u
	return nil
}
func (m *memoryStore) RecordVerificationFailure(_ context.Context, id uuid.UUID, now time.Time) error {
	u := m.users[id]
	if u.VerificationWindowStartedAt == nil || !now.Before(u.VerificationWindowStartedAt.Add(15*time.Minute)) {
		u.VerificationFailures = 0
		u.VerificationWindowStartedAt = &now
	}
	u.VerificationFailures++
	m.users[id] = u
	return nil
}
func (m *memoryStore) FindContext(_ context.Context, hash []byte, _ bool) (domain.SecurityContext, error) {
	c, ok := m.contexts[string(hash)]
	if !ok {
		return domain.SecurityContext{}, domain.ErrNotFound
	}
	return c, nil
}
func (m *memoryStore) InsertContext(_ context.Context, c domain.SecurityContext) error {
	m.contexts[string(c.TokenHash)] = c
	return nil
}
func (m *memoryStore) RevokeContext(_ context.Context, id uuid.UUID, now time.Time) error {
	for hash, c := range m.contexts {
		if c.ID == id {
			c.RevokedAt = &now
			m.contexts[hash] = c
		}
	}
	return nil
}
func (m *memoryStore) TouchSession(_ context.Context, id uuid.UUID, now time.Time) error {
	for hash, c := range m.contexts {
		if c.ID == id {
			c.LastSeenAt = now
			m.contexts[hash] = c
		}
	}
	return nil
}
func (m *memoryStore) FindChallenge(_ context.Context, id uuid.UUID, _ bool) (domain.Challenge, error) {
	c, ok := m.challenges[id]
	if !ok {
		return domain.Challenge{}, domain.ErrNotFound
	}
	return c, nil
}
func (m *memoryStore) RevokeChallenges(_ context.Context, id uuid.UUID, now time.Time) error {
	for key, c := range m.challenges {
		if c.UserID == id && c.RevokedAt == nil {
			c.RevokedAt = &now
			m.challenges[key] = c
		}
	}
	return nil
}
func (m *memoryStore) InsertChallenge(_ context.Context, c domain.Challenge) error {
	m.challenges[c.ID] = c
	return nil
}
func (m *memoryStore) SaveChallenge(_ context.Context, c domain.Challenge) error {
	m.challenges[c.ID] = c
	return nil
}
func (m *memoryStore) InsertEvidence(_ context.Context, e domain.ConsentEvidence) error {
	if m.failEvidence {
		return domain.ErrUnavailable
	}
	m.evidence = append(m.evidence, e)
	return nil
}
func (m *memoryStore) FindMembership(_ context.Context, company, user uuid.UUID) (domain.Membership, error) {
	member, ok := m.memberships[company]
	if !ok || member.UserID != user {
		return domain.Membership{}, domain.ErrNotFound
	}
	return member, nil
}
func (m *memoryStore) ListMemberships(_ context.Context, user, after uuid.UUID, created *time.Time, limit int) ([]domain.Membership, error) {
	var result []domain.Membership
	for _, m := range m.memberships {
		if m.UserID == user && (created == nil || m.CreatedAt.Before(*created) || (m.CreatedAt.Equal(*created) && m.CompanyID.String() < after.String())) {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CompanyID.String() > result[j].CompanyID.String()
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

type verifyFixture struct {
	service   *service
	store     *memoryStore
	user      domain.User
	challenge domain.Challenge
	write     WriteContext
	now       time.Time
	options   Options
}

func fixture(t *testing.T, demo bool) *verifyFixture {
	t.Helper()
	f := &verifyFixture{store: freshStore(), now: time.Date(2026, time.October, 24, 10, 0, 0, 0, time.UTC)}
	text := "MOCK consent fixture"
	sum := sha256.Sum256([]byte(text))
	f.options = Options{CodeKey: bytes.Repeat([]byte{1}, 32), CSRFKey: bytes.Repeat([]byte{2}, 32), Demo: demo, DemoEmails: []string{"owner@example.test", "other@example.test"}, Consent: Document{Text: text, Version: "test-1", SHA256: hex.EncodeToString(sum[:])}, Clock: func() time.Time { return f.now }}
	var err error
	f.store.document = f.options.Consent
	f.service, err = New(f.store.work, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.user = domain.User{ID: uuid.New(), Email: "owner@example.test", State: "active", CredentialVersion: 1, EmailVerificationMethod: "strict"}
	f.store.users[f.user.ID] = f.user
	c, token, err := f.service.newContext(domain.User{})
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(c.TokenHash)] = c
	f.write = WriteContext{CookieToken: token, CSRFToken: f.service.csrfToken(c)}
	f.challenge = domain.Challenge{ID: uuid.New(), UserID: f.user.ID, Purpose: "email_verification", CodeKeyID: "primary", CreatedAt: f.now, ExpiresAt: f.now.Add(10 * time.Minute)}
	f.challenge.CodeHMAC = codeMAC(f.options.CodeKey, f.challenge.ID, f.user.ID, "123456")
	f.store.challenges[f.challenge.ID] = f.challenge
	return f
}
func (f *verifyFixture) verify(t *testing.T, code string, want error) {
	t.Helper()
	err := f.service.VerifyEmail(t.Context(), VerifyEmailInput{Context: f.write, ChallengeID: f.challenge.ID, Code: code})
	if !errors.Is(err, want) {
		t.Fatalf("verify got %v, want %v", err, want)
	}
}
func TestDemoVerificationConsumesBoundChallengeWithoutSession(t *testing.T) {
	f := fixture(t, true)
	for _, code := range []string{"12345", "1234567", "１２３４５６", "١٢٣٤٥٦", "12345x"} {
		f.verify(t, code, ErrValidation)
	}
	f.verify(t, "000000", nil)
	user := f.store.users[f.user.ID]
	if !user.EmailVerified || user.EmailVerificationMethod != "demo" || f.store.challenges[f.challenge.ID].ConsumedAt == nil {
		t.Fatal("demo proof not recorded")
	}
	if len(f.store.contexts) != 1 {
		t.Fatal("verification created a new context")
	}
	for _, c := range f.store.contexts {
		if c.Kind != "pre_auth" {
			t.Fatal("verification logged in")
		}
	}
	f.verify(t, "999999", nil)
	if f.store.users[f.user.ID].VerificationFailures != 0 {
		t.Fatal("valid demo value counted as failure")
	}
}
func TestStrictBudgetSurvivesResendAndErrorsCommit(t *testing.T) {
	f := fixture(t, false)
	for range 5 {
		f.verify(t, "999999", ErrCodeInvalid)
		next, err := f.service.ResendVerification(t.Context(), f.write, f.user.Email)
		if err != nil {
			t.Fatal(err)
		}
		f.challenge = f.store.challenges[next.ID]
		f.challenge.CodeHMAC = codeMAC(f.options.CodeKey, f.challenge.ID, f.user.ID, "123456")
		f.store.challenges[next.ID] = f.challenge
	}
	if f.store.users[f.user.ID].VerificationFailures != 5 {
		t.Fatal("failed attempts rolled back or reset by resend")
	}
	f.verify(t, "123456", ErrCodeInvalid)
	if f.store.users[f.user.ID].EmailVerified {
		t.Fatal("blocked user verified")
	}
}
func TestVerifyEnforcesChallengeStateActorAndCSRF(t *testing.T) {
	for _, scenario := range []string{"expired", "revoked", "blocked", "foreign actor", "csrf"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t, true)
			want := ErrCodeInvalid
			switch scenario {
			case "expired":
				f.now = f.challenge.ExpiresAt
			case "revoked":
				now := f.now
				f.challenge.RevokedAt = &now
				f.store.challenges[f.challenge.ID] = f.challenge
			case "blocked":
				f.challenge.FailedAttempts = 5
				f.store.challenges[f.challenge.ID] = f.challenge
			case "foreign actor":
				other := domain.User{ID: uuid.New(), Email: "other@example.test", State: "active", CredentialVersion: 1}
				f.store.users[other.ID] = other
				c, token, err := f.service.newContext(other)
				if err != nil {
					t.Fatal(err)
				}
				f.store.contexts[string(c.TokenHash)] = c
				f.write = WriteContext{CookieToken: token, CSRFToken: f.service.csrfToken(c)}
			case "csrf":
				f.write.CSRFToken = "bad"
				want = ErrCSRFInvalid
			}
			// Keep pre-auth alive when checking the exact challenge expiry boundary.
			if scenario == "expired" {
				c, token, err := f.service.newContext(domain.User{})
				if err != nil {
					t.Fatal(err)
				}
				f.store.contexts[string(c.TokenHash)] = c
				f.write = WriteContext{CookieToken: token, CSRFToken: f.service.csrfToken(c)}
			}
			f.verify(t, "000000", want)
			if f.store.users[f.user.ID].EmailVerified || f.store.challenges[f.challenge.ID].FailedAttempts != f.challenge.FailedAttempts {
				t.Fatal("rejected request mutated another user's proof")
			}
		})
	}
}
func TestDemoProofCannotGrantStrictCompanyAccess(t *testing.T) {
	f := fixture(t, true)
	f.verify(t, "000000", nil)
	user := f.store.users[f.user.ID]
	company := uuid.Must(uuid.NewV7())
	f.store.memberships[company] = domain.Membership{CompanyID: company, UserID: user.ID, Role: "owner", CreatedAt: f.now}
	c, token, err := f.service.newContext(user)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(c.TokenHash)] = c
	if _, err := f.service.AuthorizeCompany(t.Context(), AuthorizeCompanyInput{CookieToken: token, CompanyID: company, Permission: "financial_read"}); err != nil {
		t.Fatal(err)
	}
	f.options.Demo = false
	strict, err := New(f.store.work, f.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := strict.GetCurrentAccount(t.Context(), token)
	if err != nil || got.EmailVerified {
		t.Fatal("strict mode treated demo as real email proof")
	}
	if _, err := strict.AuthorizeCompany(t.Context(), AuthorizeCompanyInput{CookieToken: token, CompanyID: company, Permission: "financial_read"}); !errors.Is(err, ErrEmailUnverified) {
		t.Fatal("demo proof retained strict company access")
	}
}
func TestCodeMACIsBoundToPurposeChallengeAndUser(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	challenge, user := uuid.New(), uuid.New()
	want := codeMAC(key, challenge, user, "000001")
	for _, got := range [][]byte{codeMAC(key, uuid.New(), user, "000001"), codeMAC(key, challenge, uuid.New(), "000001"), codeMAC(key, challenge, user, "100001")} {
		if bytes.Equal(want, got) {
			t.Fatal("MAC is not bound to request")
		}
	}
}
func TestHashQueueCancellationAndPasswordSemantics(t *testing.T) {
	f := fixture(t, false)
	f.service.hashSlots <- struct{}{}
	f.service.hashSlots <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.service.hashPassword(ctx, "password-with-spaces "); !errors.Is(err, context.Canceled) {
		t.Fatalf("hash queue: %v", err)
	}
	<-f.service.hashSlots
	<-f.service.hashSlots
	encoded, err := f.service.hashPassword(t.Context(), " password12345 ")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := comparePassword(encoded, " password12345 "); err != nil || !ok {
		t.Fatal("original password failed")
	}
	if ok, err := comparePassword(encoded, "password12345"); err != nil || ok {
		t.Fatal("password was trimmed")
	}
}

func (m *memoryStore) GetActiveDocument(_ context.Context, _ bool) (domain.ConsentDocument, error) {
	return m.document, nil
}

func TestCSRFMatchesPublishedWireFormula(t *testing.T) {
	s := &service{csrfKey: bytes.Repeat([]byte{2}, 32)}
	c := domain.SecurityContext{ID: uuid.MustParse("0192b9be-1234-4000-8000-000000000001"), Kind: "pre_auth", CSRFNonce: bytes.Repeat([]byte{3}, 32)}
	if got := s.csrfToken(c); got != "1nbRSY_Ag4sRtidAaDhKNN4q574Dn1113oVw9yEBs1w" {
		t.Fatalf("CSRF wire formula mismatch: %s", got)
	}
}
func TestMembershipCursorUsesDescendingTimeAndActorBinding(t *testing.T) {
	f := fixture(t, false)
	user := f.user
	user.EmailVerified = true
	f.store.users[user.ID] = user
	c, cookie, err := f.service.newContext(user)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(c.TokenHash)] = c
	older := uuid.MustParse("0192b9be-1234-7000-8000-000000000001")
	newerLow := uuid.MustParse("0192b9be-1234-7000-8000-000000000002")
	newerHigh := uuid.MustParse("0192b9be-1234-7000-8000-000000000003")
	for _, id := range []uuid.UUID{older, newerLow, newerHigh} {
		created := f.now
		if id == older {
			created = created.Add(-time.Hour)
		}
		f.store.memberships[id] = domain.Membership{CompanyID: id, UserID: user.ID, Role: "owner", CreatedAt: created}
	}
	page, err := f.service.ListMemberships(t.Context(), cookie, 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].CompanyID != newerHigh || page.NextCursor == "" {
		t.Fatal("first page not in created_at/ID descending order")
	}
	second, err := f.service.ListMemberships(t.Context(), cookie, 1, page.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].CompanyID != newerLow {
		t.Fatal("keyset cursor skipped or repeated a row")
	}
	other := domain.User{ID: uuid.New(), Email: "other@example.test", State: "active", CredentialVersion: 1, EmailVerified: true, EmailVerificationMethod: "strict"}
	f.store.users[other.ID] = other
	otherContext, otherCookie, err := f.service.newContext(other)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(otherContext.TokenHash)] = otherContext
	if _, err := f.service.ListMemberships(t.Context(), otherCookie, 1, page.NextCursor); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("foreign actor cursor accepted")
	}
	if _, err := f.service.ListMemberships(t.Context(), cookie, 1, "bad.cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("forged cursor accepted")
	}
}
