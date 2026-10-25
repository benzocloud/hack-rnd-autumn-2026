package account

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *service) AuthorizeCompany(ctx context.Context, input AuthorizeCompanyInput) (Authorization, error) {
	mutation := false
	switch input.Permission {
	case "read", "financial_read", "members_read":
	case "update", "financial_write", "members_manage", "invitations_manage":
		mutation = true
	default:
		return Authorization{}, ErrValidation
	}
	if input.CompanyID == uuid.Nil {
		return Authorization{}, ErrValidation
	}
	if mutation && !input.Write {
		return Authorization{}, ErrCSRFInvalid
	}
	var result Authorization
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, _ invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: input.CookieToken, CSRFToken: input.CSRFToken}, mutation || input.Write, true)
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
		if membership.Role != "owner" && (membership.Role != "financier" || (input.Permission != "read" && input.Permission != "financial_read" && input.Permission != "financial_write")) {
			return ErrForbidden
		}
		result = Authorization{ActorID: user.ID, Role: membership.Role}
		return nil
	})
	if err != nil {
		return Authorization{}, err
	}
	return result, nil
}
func (s *service) ListMemberships(ctx context.Context, cookie string, limit int, cursor string) (MembershipPage, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return MembershipPage{}, ErrValidation
	}
	if len(cursor) > 1024 {
		return MembershipPage{}, ErrInvalidCursor
	}
	result := MembershipPage{Items: []Membership{}}
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, memberships membershipStore, _ creationStore, _ invitationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, WriteContext{CookieToken: cookie}, false, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		after, afterCreated, err := s.parseMembershipCursor(user.ID, cursor)
		if err != nil {
			return err
		}
		rows, err := memberships.ListMemberships(ctx, user.ID, after, afterCreated, limit+1)
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			result.Items = append(result.Items, Membership{CompanyID: row.CompanyID, Role: row.Role, CreatedAt: row.CreatedAt})
		}
		if more {
			result.NextCursor = s.membershipCursor(user.ID, rows[len(rows)-1].CompanyID, rows[len(rows)-1].CreatedAt)
		}
		return nil
	})
	if err != nil {
		return MembershipPage{}, err
	}
	return result, nil
}
func (s *service) membershipCursor(actor, company uuid.UUID, created time.Time) string {
	payload := company.String() + "|" + created.UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, s.csrfKey)
	_, _ = mac.Write([]byte("memberships\x00" + actor.String() + "\x00" + payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *service) parseMembershipCursor(actor uuid.UUID, cursor string) (uuid.UUID, *time.Time, error) {
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
	if !hmac.Equal([]byte(cursor), []byte(s.membershipCursor(actor, after, created))) {
		return uuid.Nil, nil, ErrInvalidCursor
	}
	return after, &created, nil
}
