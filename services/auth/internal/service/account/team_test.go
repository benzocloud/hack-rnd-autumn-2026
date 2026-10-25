package account

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func TestTeamInvitationLifecycle(t *testing.T) {
	f := fixture(t, true)
	f.verify(t, "000000", nil)
	owner := f.store.users[f.user.ID]
	owner.EmailVerified = true
	owner.EmailVerificationMethod = "demo"
	f.store.users[owner.ID] = owner
	company := uuid.Must(uuid.NewV7())
	f.store.memberships[membershipKey(company, owner.ID)] = domain.Membership{CompanyID: company, UserID: owner.ID, Role: "owner", CreatedAt: f.now}

	financier := domain.User{ID: uuid.New(), Email: "financier@example.test", DisplayName: "Fin", State: "active", CredentialVersion: 1, EmailVerified: true, EmailVerificationMethod: "demo"}
	f.store.users[financier.ID] = financier
	f.options.DemoEmails = append(f.options.DemoEmails, financier.Email)
	svc, err := New(f.store.work, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc

	ownerCtx, ownerCookie, err := f.service.newContext(owner)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(ownerCtx.TokenHash)] = ownerCtx
	ownerWrite := WriteContext{CookieToken: ownerCookie, CSRFToken: f.service.csrfToken(ownerCtx)}

	created, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "Financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "invite-key-1"})
	if err != nil || created.Email != "financier@example.test" || created.Status != "pending" || created.Role != "financier" {
		t.Fatal(created, err)
	}
	if !created.ExpiresAt.Equal(f.now.Add(invitationLifetime)) {
		t.Fatal("lifetime is not seven days", created.ExpiresAt)
	}
	replay, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "invite-key-1"})
	if err != nil || replay.ID != created.ID || !replay.ExpiresAt.Equal(created.ExpiresAt) {
		t.Fatal("create idempotency failed", replay, err)
	}
	if _, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "other@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "invite-key-1"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}

	finCtx, finCookie, err := f.service.newContext(financier)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(finCtx.TokenHash)] = finCtx
	finWrite := WriteContext{CookieToken: finCookie, CSRFToken: f.service.csrfToken(finCtx)}

	incoming, err := f.service.ListIncomingInvitations(t.Context(), ListIncomingInvitationsInput{CookieToken: finCookie, Limit: 10})
	if err != nil || len(incoming.Items) != 1 || incoming.Items[0].ID != created.ID {
		t.Fatal(incoming, err)
	}

	companyID, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: "1", IdempotencyKey: "accept-key-1"})
	if err != nil || companyID != company {
		t.Fatal(companyID, err)
	}
	member, err := f.store.FindMembership(t.Context(), company, financier.ID)
	if err != nil || member.Role != "financier" {
		t.Fatal(member, err)
	}
	replayAccept, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: "1", IdempotencyKey: "accept-key-1"})
	if err != nil || replayAccept != company {
		t.Fatal(replayAccept, err)
	}
	if _, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: "2", IdempotencyKey: "accept-key-1"}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}

	if err := f.service.RemoveMember(t.Context(), RemoveMemberInput{Context: ownerWrite, CompanyID: company, UserID: financier.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.FindMembership(t.Context(), company, financier.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: "1", IdempotencyKey: "accept-key-1"}); !errors.Is(err, ErrMembershipRevoked) {
		t.Fatal("accept replay restored membership", err)
	}
	if err := f.service.RemoveMember(t.Context(), RemoveMemberInput{Context: ownerWrite, CompanyID: company, UserID: owner.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}

func TestTeamDeclineRevokeExpiry(t *testing.T) {
	f := fixture(t, true)
	f.verify(t, "000000", nil)
	owner := f.store.users[f.user.ID]
	owner.EmailVerified = true
	owner.EmailVerificationMethod = "demo"
	f.store.users[owner.ID] = owner
	company := uuid.Must(uuid.NewV7())
	f.store.memberships[membershipKey(company, owner.ID)] = domain.Membership{CompanyID: company, UserID: owner.ID, Role: "owner", CreatedAt: f.now}
	recipient := domain.User{ID: uuid.New(), Email: "guest@example.test", DisplayName: "Guest", State: "active", CredentialVersion: 1, EmailVerified: true, EmailVerificationMethod: "demo"}
	f.store.users[recipient.ID] = recipient
	f.options.DemoEmails = append(f.options.DemoEmails, recipient.Email)
	svc, err := New(f.store.work, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc
	ownerCtx, ownerCookie, err := f.service.newContext(owner)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(ownerCtx.TokenHash)] = ownerCtx
	ownerWrite := WriteContext{CookieToken: ownerCookie, CSRFToken: f.service.csrfToken(ownerCtx)}
	invite, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: recipient.Email, CompanyNameSnapshot: "Shop", IdempotencyKey: "decline-key-1"})
	if err != nil {
		t.Fatal(err)
	}
	recCtx, recCookie, err := f.service.newContext(recipient)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(recCtx.TokenHash)] = recCtx
	recWrite := WriteContext{CookieToken: recCookie, CSRFToken: f.service.csrfToken(recCtx)}
	declined, err := f.service.DeclineInvitation(t.Context(), DeclineInvitationInput{Context: recWrite, InvitationID: invite.ID, Version: "1"})
	if err != nil || declined.Status != "declined" {
		t.Fatal(declined, err)
	}
	if _, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: recWrite, InvitationID: invite.ID, Version: "2", IdempotencyKey: "accept-after-decline"}); !errors.Is(err, ErrInvitationStateConflict) {
		t.Fatal(err)
	}

	second, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: recipient.Email, CompanyNameSnapshot: "Shop", IdempotencyKey: "revoke-key-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.RevokeInvitation(t.Context(), RevokeInvitationInput{Context: ownerWrite, CompanyID: company, InvitationID: second.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: recWrite, InvitationID: second.ID, Version: "1", IdempotencyKey: "accept-after-revoke"}); !errors.Is(err, ErrInvitationStateConflict) {
		t.Fatal(err)
	}

	third, err := f.service.CreateInvitation(t.Context(), CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: recipient.Email, CompanyNameSnapshot: "Shop", IdempotencyKey: "expire-key-1"})
	if err != nil {
		t.Fatal(err)
	}
	stored := f.store.invitations[third.ID]
	stored.ExpiresAt = f.now.Add(-time.Second)
	f.store.invitations[third.ID] = stored
	if _, err := f.service.AcceptInvitation(t.Context(), AcceptInvitationInput{Context: recWrite, InvitationID: third.ID, Version: "1", IdempotencyKey: "accept-after-expire"}); !errors.Is(err, ErrInvitationStateConflict) {
		t.Fatal(err)
	}
}

func TestTeamMembersPaginationAndScope(t *testing.T) {
	f := fixture(t, true)
	f.verify(t, "000000", nil)
	owner := f.store.users[f.user.ID]
	owner.EmailVerified = true
	owner.EmailVerificationMethod = "demo"
	f.store.users[owner.ID] = owner
	company := uuid.Must(uuid.NewV7())
	f.store.memberships[membershipKey(company, owner.ID)] = domain.Membership{CompanyID: company, UserID: owner.ID, Role: "owner", CreatedAt: f.now}
	fin := domain.User{ID: uuid.New(), Email: "fin2@example.test", DisplayName: "F2", State: "active", CredentialVersion: 1, EmailVerified: true, EmailVerificationMethod: "demo"}
	f.store.users[fin.ID] = fin
	f.options.DemoEmails = append(f.options.DemoEmails, fin.Email)
	svc, err := New(f.store.work, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc
	f.store.memberships[membershipKey(company, fin.ID)] = domain.Membership{CompanyID: company, UserID: fin.ID, Role: "financier", CreatedAt: f.now.Add(-time.Minute)}
	ownerCtx, ownerCookie, err := f.service.newContext(owner)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(ownerCtx.TokenHash)] = ownerCtx
	page, err := f.service.ListMembers(t.Context(), ListMembersInput{CookieToken: ownerCookie, CompanyID: company, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].UserID != owner.ID || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	second, err := f.service.ListMembers(t.Context(), ListMembersInput{CookieToken: ownerCookie, CompanyID: company, Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].UserID != fin.ID {
		t.Fatal(second, err)
	}
	finCtx, finCookie, err := f.service.newContext(fin)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(finCtx.TokenHash)] = finCtx
	if _, err := f.service.ListMembers(t.Context(), ListMembersInput{CookieToken: finCookie, CompanyID: company, Limit: 10}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := f.service.ListMembers(t.Context(), ListMembersInput{CookieToken: ownerCookie, CompanyID: company, Limit: 1, Cursor: page.NextCursor + "x"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal(err)
	}
}
