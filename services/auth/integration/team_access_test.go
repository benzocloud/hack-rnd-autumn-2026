//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type teamOperations interface {
	accountOperations
	ListMembers(context.Context, account.ListMembersInput) (account.MemberPage, error)
	RemoveMember(context.Context, account.RemoveMemberInput) error
	CreateInvitation(context.Context, account.CreateInvitationInput) (account.Invitation, error)
	ListOutgoingInvitations(context.Context, account.ListOutgoingInvitationsInput) (account.InvitationPage, error)
	ListIncomingInvitations(context.Context, account.ListIncomingInvitationsInput) (account.InvitationPage, error)
	AcceptInvitation(context.Context, account.AcceptInvitationInput) (uuid.UUID, error)
	DeclineInvitation(context.Context, account.DeclineInvitationInput) (account.Invitation, error)
	RevokeInvitation(context.Context, account.RevokeInvitationInput) error
}

func teamFixture(t *testing.T) (config.Config, *pgx.Conn, teamOperations) {
	t.Helper()
	_, db := scratch(t)
	cfg := accountConfiguration(t, db, true)
	cfg.DemoTestEmails = append([]string{}, cfg.DemoTestEmails...)
	cfg.DemoTestEmails = append(cfg.DemoTestEmails, "owner@example.test", "financier@example.test", "stranger@example.test")
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	application, err := app.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return cfg, db, application.Account
}

func TestTeamAccessDatabaseCycle(t *testing.T) {
	cfg, db, service := teamFixture(t)
	ownerChallenge, ownerBrowser := registerAccount(t, service, "owner@example.test")
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(ownerBrowser), ChallengeID: ownerChallenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	owner := loginAccount(t, service, ownerBrowser, "owner@example.test")
	company := uuid.Must(uuid.NewV7())
	execAccountFixture(t, db, "INSERT INTO memberships(company_id,user_id,role) VALUES($1,$2,'owner')", company, owner.User.ID)

	finChallenge, finBrowser := registerAccount(t, service, "financier@example.test")
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(finBrowser), ChallengeID: finChallenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	financier := loginAccount(t, service, finBrowser, "financier@example.test")

	strangerChallenge, strangerBrowser := registerAccount(t, service, "stranger@example.test")
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(strangerBrowser), ChallengeID: strangerChallenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	stranger := loginAccount(t, service, strangerBrowser, "stranger@example.test")

	ownerWrite := accountWrite(owner.Context)
	members, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: owner.Context.CookieToken, CompanyID: company, Limit: 10})
	if err != nil || len(members.Items) != 1 || members.Items[0].Role != "owner" {
		t.Fatal(members, err)
	}
	if _, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: financier.Context.CookieToken, CompanyID: company, Limit: 10}); !errors.Is(err, account.ErrNotFound) {
		t.Fatal("non-member listed members", err)
	}
	if _, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: stranger.Context.CookieToken, CompanyID: company, Limit: 10}); !errors.Is(err, account.ErrNotFound) {
		t.Fatal("stranger listed members", err)
	}
	if _, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: owner.Context.CookieToken, CompanyID: company, Limit: 10, Cursor: "forged.cursor"}); !errors.Is(err, account.ErrInvalidCursor) {
		t.Fatal(err)
	}

	created, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "Financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-1"})
	if err != nil || created.Status != "pending" || created.Email != "financier@example.test" {
		t.Fatal(created, err)
	}
	replay, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-1"})
	if err != nil || replay.ID != created.ID {
		t.Fatal(replay, err)
	}
	if _, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "stranger@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-1"}); !errors.Is(err, account.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	outgoing, err := service.ListOutgoingInvitations(t.Context(), account.ListOutgoingInvitationsInput{CookieToken: owner.Context.CookieToken, CompanyID: company, Limit: 10})
	if err != nil || len(outgoing.Items) != 1 {
		t.Fatal(outgoing, err)
	}
	incoming, err := service.ListIncomingInvitations(t.Context(), account.ListIncomingInvitationsInput{CookieToken: financier.Context.CookieToken, Limit: 10})
	if err != nil || len(incoming.Items) != 1 || incoming.Items[0].ID != created.ID {
		t.Fatal(incoming, err)
	}
	if foreign, err := service.ListIncomingInvitations(t.Context(), account.ListIncomingInvitationsInput{CookieToken: stranger.Context.CookieToken, Limit: 10}); err != nil || len(foreign.Items) != 0 {
		t.Fatal(foreign, err)
	}

	badCSRF := account.WriteContext{CookieToken: financier.Context.CookieToken, CSRFToken: "not-the-real-csrf-token-value"}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: badCSRF, InvitationID: created.ID, Version: "1", IdempotencyKey: "db-accept-1"}); !errors.Is(err, account.ErrCSRFInvalid) {
		t.Fatal(err)
	}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: accountWrite(stranger.Context), InvitationID: created.ID, Version: "1", IdempotencyKey: "db-accept-stranger"}); !errors.Is(err, account.ErrNotFound) {
		t.Fatal(err)
	}

	finWrite := accountWrite(financier.Context)
	acceptedCompany, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: strconv.FormatInt(created.Version, 10), IdempotencyKey: "db-accept-1"})
	if err != nil || acceptedCompany != company {
		t.Fatal(acceptedCompany, err)
	}
	replayAccept, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: strconv.FormatInt(created.Version, 10), IdempotencyKey: "db-accept-1"})
	if err != nil || replayAccept != company {
		t.Fatal(replayAccept, err)
	}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: "99", IdempotencyKey: "db-accept-1"}); !errors.Is(err, account.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	authz, err := service.AuthorizeCompany(t.Context(), account.AuthorizeCompanyInput{CookieToken: financier.Context.CookieToken, CompanyID: company, Permission: "financial_read"})
	if err != nil || authz.Role != "financier" {
		t.Fatal(authz, err)
	}
	if _, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: financier.Context.CookieToken, CompanyID: company, Limit: 10}); !errors.Is(err, account.ErrForbidden) {
		t.Fatal("financier listed members", err)
	}
	listed, err := service.ListMembers(t.Context(), account.ListMembersInput{CookieToken: owner.Context.CookieToken, CompanyID: company, Limit: 10})
	if err != nil || len(listed.Items) != 2 {
		t.Fatal(listed, err)
	}

	if err := service.RemoveMember(t.Context(), account.RemoveMemberInput{Context: ownerWrite, CompanyID: company, UserID: financier.User.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeCompany(t.Context(), account.AuthorizeCompanyInput{CookieToken: financier.Context.CookieToken, CompanyID: company, Permission: "financial_read"}); !errors.Is(err, account.ErrNotFound) {
		t.Fatal("removed financier retained access", err)
	}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: created.ID, Version: strconv.FormatInt(created.Version, 10), IdempotencyKey: "db-accept-1"}); !errors.Is(err, account.ErrMembershipRevoked) {
		t.Fatal("accept replay restored membership", err)
	}
	if err := service.RemoveMember(t.Context(), account.RemoveMemberInput{Context: ownerWrite, CompanyID: company, UserID: owner.User.ID}); !errors.Is(err, account.ErrForbidden) {
		t.Fatal(err)
	}

	second, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeInvitation(t.Context(), account.RevokeInvitationInput{Context: ownerWrite, CompanyID: company, InvitationID: second.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: second.ID, Version: "1", IdempotencyKey: "db-accept-revoked"}); !errors.Is(err, account.ErrInvitationStateConflict) {
		t.Fatal(err)
	}

	third, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-3"})
	if err != nil {
		t.Fatal(err)
	}
	declined, err := service.DeclineInvitation(t.Context(), account.DeclineInvitationInput{Context: finWrite, InvitationID: third.ID, Version: "1"})
	if err != nil || declined.Status != "declined" {
		t.Fatal(declined, err)
	}

	fourth, err := service.CreateInvitation(t.Context(), account.CreateInvitationInput{Context: ownerWrite, CompanyID: company, Email: "financier@example.test", CompanyNameSnapshot: "Кофейня", IdempotencyKey: "db-invite-4"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), "UPDATE invitations SET expires_at=$2 WHERE id=$1", fourth.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptInvitation(t.Context(), account.AcceptInvitationInput{Context: finWrite, InvitationID: fourth.ID, Version: "1", IdempotencyKey: "db-accept-expired"}); !errors.Is(err, account.ErrInvitationStateConflict) {
		t.Fatal(err)
	}

	restartedApp, err := app.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restartedApp.Close)
	persisted, err := restartedApp.Account.ListOutgoingInvitations(t.Context(), account.ListOutgoingInvitationsInput{CookieToken: owner.Context.CookieToken, CompanyID: company, Limit: 50})
	if err != nil || len(persisted.Items) < 3 {
		t.Fatal("restart lost invitation history", persisted, err)
	}
}
