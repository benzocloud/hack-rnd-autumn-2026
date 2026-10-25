package account

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

const (
	invitationLifetime  = 7 * 24 * time.Hour
	invitationRetention = 30 * 24 * time.Hour
)

// Invitation is the public invitation projection for Auth RPC handlers.
type Invitation struct {
	ID                  uuid.UUID
	CompanyID           uuid.UUID
	Email               string
	CompanyNameSnapshot string
	Role                string
	Status              string
	Version             int64
	CreatedAt           time.Time
	ExpiresAt           time.Time
}

type Member struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	Role        string
	JoinedAt    time.Time
}

type MemberPage struct {
	Items      []Member
	NextCursor string
}

type InvitationPage struct {
	Items      []Invitation
	NextCursor string
}

type ListMembersInput struct {
	CookieToken string
	CompanyID   uuid.UUID
	Limit       int
	Cursor      string
}

type RemoveMemberInput struct {
	Context   WriteContext
	CompanyID uuid.UUID
	UserID    uuid.UUID
}

type CreateInvitationInput struct {
	Context             WriteContext
	CompanyID           uuid.UUID
	Email               string
	CompanyNameSnapshot string
	IdempotencyKey      string
}

type ListOutgoingInvitationsInput struct {
	CookieToken string
	CompanyID   uuid.UUID
	Limit       int
	Cursor      string
}

type ListIncomingInvitationsInput struct {
	CookieToken string
	Limit       int
	Cursor      string
}

type AcceptInvitationInput struct {
	Context        WriteContext
	InvitationID   uuid.UUID
	Version        string
	IdempotencyKey string
}

type DeclineInvitationInput struct {
	Context      WriteContext
	InvitationID uuid.UUID
	Version      string
}

type RevokeInvitationInput struct {
	Context      WriteContext
	CompanyID    uuid.UUID
	InvitationID uuid.UUID
}

func (s *service) ListMembers(ctx context.Context, input ListMembersInput) (MemberPage, error) {
	limit, err := normalizePage(input.Limit, input.Cursor)
	if err != nil {
		return MemberPage{}, err
	}
	if input.CompanyID == uuid.Nil {
		return MemberPage{}, ErrValidation
	}
	result := MemberPage{Items: []Member{}}
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, _ invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: input.CookieToken}, false, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		membership, err := memberships.FindMembership(ctx, input.CompanyID, user.ID)
		if err != nil {
			return err
		}
		if membership.Role != "owner" {
			return ErrForbidden
		}
		after, afterCreated, err := s.parseMemberCursor(user.ID, input.CompanyID, input.Cursor)
		if err != nil {
			return err
		}
		rows, err := memberships.ListCompanyMembers(ctx, input.CompanyID, after, afterCreated, limit+1)
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			result.Items = append(result.Items, Member{UserID: row.UserID, Email: row.Email, DisplayName: row.DisplayName, Role: row.Role, JoinedAt: row.JoinedAt})
		}
		if more {
			last := rows[len(rows)-1]
			result.NextCursor = s.memberCursor(user.ID, input.CompanyID, last.UserID, last.JoinedAt)
		}
		return nil
	})
	if err != nil {
		return MemberPage{}, err
	}
	return result, nil
}

func (s *service) RemoveMember(ctx context.Context, input RemoveMemberInput) error {
	if input.CompanyID == uuid.Nil || input.UserID == uuid.Nil {
		return ErrValidation
	}
	return s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, _ invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		members, err := memberships.LockCompanyMemberships(ctx, input.CompanyID)
		if err != nil {
			return err
		}
		actorRole := ""
		for _, member := range members {
			if member.UserID == user.ID {
				actorRole = member.Role
				break
			}
		}
		if actorRole == "" {
			return ErrNotFound
		}
		if actorRole != "owner" {
			return ErrForbidden
		}
		if input.UserID == user.ID {
			return ErrForbidden
		}
		for _, member := range members {
			if member.UserID == input.UserID && member.Role == "owner" {
				return ErrForbidden
			}
		}
		_, err = memberships.DeleteFinancier(ctx, input.CompanyID, input.UserID)
		return err
	})
}

func (s *service) CreateInvitation(ctx context.Context, input CreateInvitationInput) (Invitation, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Invitation{}, ErrValidation
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return Invitation{}, err
	}
	name := strings.TrimSpace(input.CompanyNameSnapshot)
	if input.CompanyID == uuid.Nil || name == "" || utf8.RuneCountInString(name) > 160 || strings.ContainsRune(name, 0) {
		return Invitation{}, ErrValidation
	}
	hash, err := invitationCreateFingerprint(email)
	if err != nil {
		return Invitation{}, ErrValidation
	}
	var result domain.Invitation
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		members, err := memberships.LockCompanyMemberships(ctx, input.CompanyID)
		if err != nil {
			return err
		}
		if !isOwner(members, user.ID) {
			if hasMember(members, user.ID) {
				return ErrForbidden
			}
			return ErrNotFound
		}
		stored, err := invitations.FindCreateIdempotency(ctx, user.ID, input.CompanyID, input.IdempotencyKey)
		if err == nil {
			if stored.CreateRequestSHA256 == nil || *stored.CreateRequestSHA256 != hash {
				return ErrIdempotencyConflict
			}
			if beyondRetention(stored, s.now()) {
				return ErrNotFound
			}
			result = stored
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		recipient, err := users.FindUserByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return ErrInvitationNotAvailable
			}
			return err
		}
		if !s.emailVerified(recipient) || hasMember(members, recipient.ID) {
			return ErrInvitationNotAvailable
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		pending, err := findPendingInvitation(ctx, invitations, input.CompanyID, email, now)
		if err != nil {
			return err
		}
		if pending != nil {
			result = *pending
			return nil
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return ErrUnavailable
		}
		key := input.IdempotencyKey
		result = domain.Invitation{
			ID: id, CompanyID: input.CompanyID, Email: email, CompanyNameSnapshot: name, Role: "financier",
			Status: "pending", Version: 1, CreatedAt: now, ExpiresAt: now.Add(invitationLifetime),
			CreatedBy: user.ID, CreateIdempotencyKey: &key, CreateRequestSHA256: &hash,
		}
		err = invitations.Insert(ctx, result)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrConflict) {
			return err
		}
		stored, lookupErr := invitations.FindCreateIdempotency(ctx, user.ID, input.CompanyID, input.IdempotencyKey)
		if lookupErr == nil {
			if stored.CreateRequestSHA256 == nil || *stored.CreateRequestSHA256 != hash {
				return ErrIdempotencyConflict
			}
			result = stored
			return nil
		}
		pending, err = findPendingInvitation(ctx, invitations, input.CompanyID, email, now)
		if err != nil {
			return err
		}
		if pending != nil {
			result = *pending
			return nil
		}
		return err
	})
	if err != nil {
		return Invitation{}, err
	}
	return publicInvitation(result), nil
}

func (s *service) ListOutgoingInvitations(ctx context.Context, input ListOutgoingInvitationsInput) (InvitationPage, error) {
	limit, err := normalizePage(input.Limit, input.Cursor)
	if err != nil {
		return InvitationPage{}, err
	}
	if input.CompanyID == uuid.Nil {
		return InvitationPage{}, ErrValidation
	}
	result := InvitationPage{Items: []Invitation{}}
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: input.CookieToken}, false, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		membership, err := memberships.FindMembership(ctx, input.CompanyID, user.ID)
		if err != nil {
			return err
		}
		if membership.Role != "owner" {
			return ErrForbidden
		}
		after, afterCreated, err := s.parseInvitationCursor("outgoing", user.ID, input.CompanyID, input.Cursor)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		rows, err := invitations.ListOutgoing(ctx, input.CompanyID, after, retentionCutoff(now), afterCreated, limit+1)
		if err != nil {
			return err
		}
		rows, err = s.materializeExpired(ctx, invitations, rows, now)
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			result.Items = append(result.Items, publicInvitation(row))
		}
		if more {
			last := rows[len(rows)-1]
			result.NextCursor = s.invitationCursor("outgoing", user.ID, input.CompanyID, last.ID, last.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return InvitationPage{}, err
	}
	return result, nil
}

func (s *service) ListIncomingInvitations(ctx context.Context, input ListIncomingInvitationsInput) (InvitationPage, error) {
	limit, err := normalizePage(input.Limit, input.Cursor)
	if err != nil {
		return InvitationPage{}, err
	}
	result := InvitationPage{Items: []Invitation{}}
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: input.CookieToken}, false, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		after, afterCreated, err := s.parseInvitationCursor("incoming", user.ID, uuid.Nil, input.Cursor)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		rows, err := invitations.ListIncoming(ctx, user.Email, after, retentionCutoff(now), afterCreated, limit+1)
		if err != nil {
			return err
		}
		rows, err = s.materializeExpired(ctx, invitations, rows, now)
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			result.Items = append(result.Items, publicInvitation(row))
		}
		if more {
			last := rows[len(rows)-1]
			result.NextCursor = s.invitationCursor("incoming", user.ID, uuid.Nil, last.ID, last.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return InvitationPage{}, err
	}
	return result, nil
}

func (s *service) AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (uuid.UUID, error) {
	if input.InvitationID == uuid.Nil {
		return uuid.Nil, ErrValidation
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return uuid.Nil, err
	}
	if !validVersionString(input.Version) {
		return uuid.Nil, ErrValidation
	}
	hash, err := invitationAcceptFingerprint(input.Version)
	if err != nil {
		return uuid.Nil, ErrValidation
	}
	var companyID uuid.UUID
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		peek, err := invitations.Find(ctx, input.InvitationID, false)
		if err != nil {
			return mapInvitationLookup(err)
		}
		if beyondRetention(peek, s.now()) || peek.Email != user.Email {
			return ErrNotFound
		}
		members, err := memberships.LockCompanyMemberships(ctx, peek.CompanyID)
		if err != nil {
			return err
		}
		invitation, err := invitations.Find(ctx, input.InvitationID, true)
		if err != nil {
			return mapInvitationLookup(err)
		}
		if beyondRetention(invitation, s.now()) || invitation.Email != user.Email || invitation.CompanyID != peek.CompanyID {
			return ErrNotFound
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		invitation, err = s.expireIfNeeded(ctx, invitations, invitation, now)
		if err != nil {
			return err
		}
		if invitation.Status == "accepted" {
			if err := acceptReplay(invitation, input.IdempotencyKey, hash); err != nil {
				return err
			}
			if !hasMember(members, user.ID) {
				return ErrMembershipRevoked
			}
			companyID = invitation.CompanyID
			return nil
		}
		if invitation.Status != "pending" {
			return ErrInvitationStateConflict
		}
		if !isOwner(members, invitation.CreatedBy) {
			return ErrInvitationNotAvailable
		}
		if hasMember(members, user.ID) {
			return ErrInvitationNotAvailable
		}
		expectedVersion, err := strconv.ParseInt(input.Version, 10, 64)
		if err != nil || expectedVersion != invitation.Version {
			return ErrVersionConflict
		}
		inserted, err := memberships.InsertFinancier(ctx, invitation.CompanyID, user.ID, now)
		if err != nil {
			return err
		}
		if !inserted {
			return ErrInvitationNotAvailable
		}
		key := input.IdempotencyKey
		invitation.Status = "accepted"
		invitation.Version++
		invitation.TerminalAt = &now
		invitation.AcceptIdempotencyKey = &key
		invitation.AcceptRequestSHA256 = &hash
		if err := invitations.Save(ctx, invitation, expectedVersion); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return ErrVersionConflict
			}
			return err
		}
		companyID = invitation.CompanyID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return companyID, nil
}

func (s *service) DeclineInvitation(ctx context.Context, input DeclineInvitationInput) (Invitation, error) {
	if input.InvitationID == uuid.Nil || !validVersionString(input.Version) {
		return Invitation{}, ErrValidation
	}
	var result domain.Invitation
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		peek, err := invitations.Find(ctx, input.InvitationID, false)
		if err != nil {
			return mapInvitationLookup(err)
		}
		if beyondRetention(peek, s.now()) || peek.Email != user.Email {
			return ErrNotFound
		}
		if _, err := memberships.LockCompanyMemberships(ctx, peek.CompanyID); err != nil {
			return err
		}
		invitation, err := invitations.Find(ctx, input.InvitationID, true)
		if err != nil {
			return mapInvitationLookup(err)
		}
		if beyondRetention(invitation, s.now()) || invitation.Email != user.Email || invitation.CompanyID != peek.CompanyID {
			return ErrNotFound
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		invitation, err = s.expireIfNeeded(ctx, invitations, invitation, now)
		if err != nil {
			return err
		}
		if invitation.Status == "declined" {
			result = invitation
			return nil
		}
		if invitation.Status != "pending" {
			return ErrInvitationStateConflict
		}
		expectedVersion, err := strconv.ParseInt(input.Version, 10, 64)
		if err != nil || expectedVersion != invitation.Version {
			return ErrVersionConflict
		}
		invitation.Status = "declined"
		invitation.Version++
		invitation.TerminalAt = &now
		if err := invitations.Save(ctx, invitation, expectedVersion); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return ErrVersionConflict
			}
			return err
		}
		result = invitation
		return nil
	})
	if err != nil {
		return Invitation{}, err
	}
	return publicInvitation(result), nil
}

func (s *service) RevokeInvitation(ctx context.Context, input RevokeInvitationInput) error {
	if input.CompanyID == uuid.Nil || input.InvitationID == uuid.Nil {
		return ErrValidation
	}
	return s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, invitations invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		members, err := memberships.LockCompanyMemberships(ctx, input.CompanyID)
		if err != nil {
			return err
		}
		if !isOwner(members, user.ID) {
			if hasMember(members, user.ID) {
				return ErrForbidden
			}
			return ErrNotFound
		}
		invitation, err := invitations.Find(ctx, input.InvitationID, true)
		if err != nil {
			return mapInvitationLookup(err)
		}
		if invitation.CompanyID != input.CompanyID || beyondRetention(invitation, s.now()) {
			return ErrNotFound
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		invitation, err = s.expireIfNeeded(ctx, invitations, invitation, now)
		if err != nil {
			return err
		}
		if invitation.Status == "revoked" {
			return nil
		}
		if invitation.Status != "pending" {
			return ErrInvitationStateConflict
		}
		expected := invitation.Version
		invitation.Status = "revoked"
		invitation.Version++
		invitation.TerminalAt = &now
		if err := invitations.Save(ctx, invitation, expected); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return ErrVersionConflict
			}
			return err
		}
		return nil
	})
}

func normalizePage(limit int, cursor string) (int, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return 0, ErrValidation
	}
	if len(cursor) > 1024 {
		return 0, ErrInvalidCursor
	}
	return limit, nil
}

func validateIdempotencyKey(key string) error {
	if len(key) < 8 || len(key) > 128 {
		return ErrValidation
	}
	for _, c := range key {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		separator := c == '.' || c == '_' || c == ':' || c == '-'
		if !letter && !digit && !separator {
			return ErrValidation
		}
	}
	return nil
}

func validVersionString(version string) bool {
	if version == "" || version[0] == '0' {
		return false
	}
	for _, c := range version {
		if c < '0' || c > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(version, 10, 64)
	return err == nil
}

func invitationCreateFingerprint(email string) (string, error) {
	payload, err := json.Marshal(struct {
		Email string `json:"email"`
	}{Email: email})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func invitationAcceptFingerprint(version string) (string, error) {
	payload, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: version})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func publicInvitation(row domain.Invitation) Invitation {
	return Invitation{
		ID: row.ID, CompanyID: row.CompanyID, Email: row.Email, CompanyNameSnapshot: row.CompanyNameSnapshot,
		Role: row.Role, Status: row.Status, Version: row.Version, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
	}
}

func retentionCutoff(now time.Time) time.Time {
	return now.Add(-invitationRetention)
}

func beyondRetention(invitation domain.Invitation, now time.Time) bool {
	if invitation.TerminalAt == nil {
		return false
	}
	return !invitation.TerminalAt.After(retentionCutoff(now))
}

func isOwner(members []domain.Membership, user uuid.UUID) bool {
	for _, member := range members {
		if member.UserID == user && member.Role == "owner" {
			return true
		}
	}
	return false
}

func hasMember(members []domain.Membership, user uuid.UUID) bool {
	for _, member := range members {
		if member.UserID == user {
			return true
		}
	}
	return false
}

func mapInvitationLookup(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func acceptReplay(invitation domain.Invitation, key, hash string) error {
	if invitation.AcceptIdempotencyKey == nil {
		return ErrInvitationStateConflict
	}
	if *invitation.AcceptIdempotencyKey != key {
		return nil
	}
	if invitation.AcceptRequestSHA256 == nil || *invitation.AcceptRequestSHA256 != hash {
		return ErrIdempotencyConflict
	}
	return nil
}

func findPendingInvitation(ctx context.Context, invitations invitationStore, company uuid.UUID, email string, now time.Time) (*domain.Invitation, error) {
	rows, err := invitations.ListOutgoing(ctx, company, uuid.Nil, retentionCutoff(now), nil, 100)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row := rows[i]
		if row.Status == "pending" && row.Email == email && row.ExpiresAt.After(now) {
			return &row, nil
		}
	}
	return nil, nil
}

func (s *service) expireIfNeeded(ctx context.Context, invitations invitationStore, invitation domain.Invitation, now time.Time) (domain.Invitation, error) {
	if invitation.Status != "pending" || invitation.ExpiresAt.After(now) {
		return invitation, nil
	}
	expected := invitation.Version
	terminal := invitation.ExpiresAt
	invitation.Status = "expired"
	invitation.Version++
	invitation.TerminalAt = &terminal
	if err := invitations.Save(ctx, invitation, expected); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return invitations.Find(ctx, invitation.ID, true)
		}
		return domain.Invitation{}, err
	}
	return invitation, nil
}

func (s *service) materializeExpired(ctx context.Context, invitations invitationStore, rows []domain.Invitation, now time.Time) ([]domain.Invitation, error) {
	out := make([]domain.Invitation, 0, len(rows))
	for _, row := range rows {
		updated, err := s.expireIfNeeded(ctx, invitations, row, now)
		if err != nil {
			return nil, err
		}
		if beyondRetention(updated, now) {
			continue
		}
		out = append(out, updated)
	}
	return out, nil
}

func (s *service) memberCursor(actor, company, user uuid.UUID, joined time.Time) string {
	payload := user.String() + "|" + joined.UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, s.csrfKey)
	_, _ = mac.Write([]byte("members\x00" + actor.String() + "\x00" + company.String() + "\x00" + payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *service) parseMemberCursor(actor, company uuid.UUID, cursor string) (uuid.UUID, *time.Time, error) {
	if cursor == "" {
		return uuid.Nil, nil, nil
	}
	parts := strings.Split(cursor, ".")
	if len(parts) != 2 {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	keys := strings.Split(string(payload), "|")
	if len(keys) != 2 {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	after, err := uuid.Parse(keys[0])
	if err != nil || after == uuid.Nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	created, err := time.Parse(time.RFC3339Nano, keys[1])
	if err != nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	if !hmac.Equal([]byte(cursor), []byte(s.memberCursor(actor, company, after, created))) {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	return after, &created, nil
}

func (s *service) invitationCursor(kind string, actor, company, id uuid.UUID, created time.Time) string {
	payload := id.String() + "|" + created.UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, s.csrfKey)
	_, _ = mac.Write([]byte(kind + "\x00" + actor.String() + "\x00" + company.String() + "\x00" + payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *service) parseInvitationCursor(kind string, actor, company uuid.UUID, cursor string) (uuid.UUID, *time.Time, error) {
	if cursor == "" {
		return uuid.Nil, nil, nil
	}
	parts := strings.Split(cursor, ".")
	if len(parts) != 2 {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	keys := strings.Split(string(payload), "|")
	if len(keys) != 2 {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	after, err := uuid.Parse(keys[0])
	if err != nil || after == uuid.Nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	created, err := time.Parse(time.RFC3339Nano, keys[1])
	if err != nil {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	if !hmac.Equal([]byte(cursor), []byte(s.invitationCursor(kind, actor, company, after, created))) {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	return after, &created, nil
}
