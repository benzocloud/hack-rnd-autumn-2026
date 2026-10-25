//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	creationrepo "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/companycreation"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/membership"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
	userrepo "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/migrations"
)

func creationCommand(t *testing.T) domain.CompanyCreation {
	t.Helper()
	spec := domain.CompanySpec{Name: "Кофейня <&>", Currency: "RUB", Timezone: "Europe/Moscow"}
	hash, err := spec.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	return domain.CompanyCreation{ID: uuid.New(), CompanyID: uuid.Must(uuid.NewV7()), InitiatorID: uuid.New(), Key: "create-company-key", Spec: spec, RequestSHA256: hash, State: domain.CreationPrepared, CreatedAt: now, UpdatedAt: now, NextAttemptAt: now}
}

func TestCompanyCreationStorageReplayConcurrencyAndPersistence(t *testing.T) {
	cfg, _ := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	store := creationrepo.NewRepository(pool, cfg.QueryTimeout)
	command := creationCommand(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			candidate := command
			candidate.ID = uuid.New()
			candidate.CompanyID = uuid.Must(uuid.NewV7())
			results <- store.InsertCreation(t.Context(), candidate)
		})
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(successes, conflicts)
	}
	saved, err := store.FindCreationByKey(t.Context(), command.InitiatorID, command.Key)
	if err != nil || saved.Spec != command.Spec || saved.RequestSHA256 != command.RequestSHA256 || saved.State != domain.CreationPrepared || saved.LeaseToken != uuid.Nil || saved.AccountID != uuid.Nil {
		t.Fatal(saved, err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	pool = openDatabase(t, cfg)
	store = creationrepo.NewRepository(pool, cfg.QueryTimeout)
	reopened, err := store.FindCreation(t.Context(), saved.ID, false)
	if err != nil || reopened.ID != saved.ID || reopened.CompanyID != saved.CompanyID || reopened.InitiatorID != saved.InitiatorID || reopened.Key != saved.Key || reopened.Spec != saved.Spec || !reopened.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatal(reopened, err)
	}
	// The scope is the initiator, not a global browser key.
	other := command
	other.ID = uuid.New()
	other.CompanyID = uuid.Must(uuid.NewV7())
	other.InitiatorID = uuid.New()
	if err := store.InsertCreation(t.Context(), other); err != nil {
		t.Fatal(err)
	}
}

func TestCompanyCreationAtomicOwnerAndCompletion(t *testing.T) {
	cfg, _ := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	command := creationCommand(t)
	if err := userrepo.NewRepository(pool, cfg.QueryTimeout).InsertUser(t.Context(), userrepo.User{ID: command.InitiatorID, Email: "owner@example.test", DisplayName: "Owner"}); err != nil {
		t.Fatal(err)
	}
	store := creationrepo.NewRepository(pool, cfg.QueryTimeout)
	if err := store.InsertCreation(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	finish := func(accountID uuid.UUID) error {
		return postgres.WithTx(t.Context(), pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			if _, err := userrepo.NewRepository(tx, cfg.QueryTimeout).FindUserByID(ctx, command.InitiatorID, true); err != nil {
				return err
			}
			if _, err := creationrepo.NewRepository(tx, cfg.QueryTimeout).FindCreation(ctx, command.ID, true); err != nil {
				return err
			}
			if err := membership.NewRepository(tx, cfg.QueryTimeout).InsertOwner(ctx, command.CompanyID, command.InitiatorID, command.CreatedAt); err != nil {
				return err
			}
			return creationrepo.NewRepository(tx, cfg.QueryTimeout).CompleteCreation(ctx, command.ID, accountID, command.CreatedAt, uuid.Nil)
		})
	}
	if err := finish(uuid.New()); err == nil {
		t.Fatal("non-v7 account accepted")
	}
	if _, err := membership.NewRepository(pool, cfg.QueryTimeout).FindMembership(t.Context(), command.CompanyID, command.InitiatorID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("owner leaked from rollback", err)
	}
	accountID := uuid.Must(uuid.NewV7())
	if err := finish(accountID); err != nil {
		t.Fatal(err)
	}
	saved, err := store.FindCreation(t.Context(), command.ID, false)
	if err != nil || saved.State != domain.CreationCompleted || saved.AccountID != accountID || saved.CommittedAt == nil {
		t.Fatal(saved, err)
	}
	owner, err := membership.NewRepository(pool, cfg.QueryTimeout).FindMembership(t.Context(), command.CompanyID, command.InitiatorID)
	if err != nil || owner.Role != "owner" {
		t.Fatal(owner, err)
	}
	if err := finish(accountID); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal("completed command mutated", err)
	}
	if err := store.QuarantineCreation(t.Context(), command.ID, uuid.Nil); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal(err)
	}
}

func TestCompanyCreationLeaseFencingAndDeletedInitiator(t *testing.T) {
	cfg, db := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	store := creationrepo.NewRepository(pool, cfg.QueryTimeout)
	command := creationCommand(t)
	if err := userrepo.NewRepository(pool, cfg.QueryTimeout).InsertUser(t.Context(), userrepo.User{ID: command.InitiatorID, Email: "deleted@example.test", DisplayName: "Deleted"}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertCreation(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	lease := uuid.New()
	claimed, err := store.ClaimCreation(t.Context(), lease)
	if err != nil || claimed.ID != command.ID || claimed.LeaseToken != lease || claimed.Attempts != 1 {
		t.Fatal(claimed, err)
	}
	if _, err := store.ClaimCreation(t.Context(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("second replica claimed leased command", err)
	}
	if _, err := db.Exec(t.Context(), "UPDATE company_creation_commands SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", command.ID); err != nil {
		t.Fatal(err)
	}
	newLease := uuid.New()
	reclaimed, err := store.ClaimCreation(t.Context(), newLease)
	if err != nil || reclaimed.ID != command.ID || reclaimed.LeaseToken != newLease || reclaimed.Attempts != 2 {
		t.Fatal(reclaimed, err)
	}
	if err := store.CompleteCreation(t.Context(), command.ID, uuid.Must(uuid.NewV7()), command.CreatedAt, lease); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal("stale worker completed", err)
	}
	if err := store.QuarantineCreation(t.Context(), command.ID, lease); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal("stale worker quarantined", err)
	}
	if err := store.RetryCreation(t.Context(), command.ID, lease, time.Second); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal("stale worker released lease", err)
	}
	if err := store.RetryCreation(t.Context(), command.ID, newLease, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimCreation(t.Context(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("backoff ignored", err)
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM users WHERE id=$1", command.InitiatorID); err != nil {
		t.Fatal(err)
	}
	saved, err := store.FindCreation(t.Context(), command.ID, false)
	if err != nil || saved.InitiatorID != command.InitiatorID || saved.State != domain.CreationPrepared {
		t.Fatal("deletion removed durable command", saved, err)
	}
	if err := store.QuarantineCreation(t.Context(), command.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimCreation(t.Context(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("quarantined command reclaimed", err)
	}
}

func TestCompanyCreationMigrationUpDownCompatibility(t *testing.T) {
	cfg, _ := scratch(t)
	pool := openDatabase(t, cfg)
	provider, err := goose.NewProvider(goose.DialectPostgres, pool.DB, migrations.Files, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := userrepo.NewRepository(pool, cfg.QueryTimeout).InsertUser(t.Context(), userrepo.User{ID: id, Email: "migration@example.test", DisplayName: "Keep me"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if err := postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()); !errors.Is(err, postgres.ErrSchema) {
		t.Fatal("old schema accepted", err)
	}
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	user, err := userrepo.NewRepository(pool, cfg.QueryTimeout).UserByEmail(t.Context(), "migration@example.test")
	if err != nil || user.ID != id || user.DisplayName != "Keep me" {
		t.Fatal("existing user changed", user, err)
	}
}

func TestCompanyCreationOwnerConflictsPreserveRoles(t *testing.T) {
	cfg, db := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	store := membership.NewRepository(pool, cfg.QueryTimeout)
	first, second := uuid.New(), uuid.New()
	for _, user := range []userrepo.User{{ID: first, Email: "first@example.test", DisplayName: "First"}, {ID: second, Email: "second@example.test", DisplayName: "Second"}} {
		if err := userrepo.NewRepository(pool, cfg.QueryTimeout).InsertUser(t.Context(), user); err != nil {
			t.Fatal(err)
		}
	}
	company, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	created := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.InsertOwner(t.Context(), company, first, created); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertOwner(t.Context(), company, first, created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertOwner(t.Context(), company, second, created); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("second owner accepted", err)
	}
	owner, err := store.FindMembership(t.Context(), company, first)
	if err != nil || owner.Role != "owner" || !owner.CreatedAt.Equal(created) {
		t.Fatal(owner, err)
	}
	if _, err := db.Exec(t.Context(), "INSERT INTO memberships(company_id,user_id,role) VALUES($1,$2,'financier')", other, second); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertOwner(t.Context(), other, second, created); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("existing financier promoted", err)
	}
	member, err := store.FindMembership(t.Context(), other, second)
	if err != nil || member.Role != "financier" {
		t.Fatal(member, err)
	}
}

func TestCompanyCreationRecoveryScheduleUsesDatabaseClock(t *testing.T) {
	cfg, db := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	store := creationrepo.NewRepository(pool, cfg.QueryTimeout)
	command := creationCommand(t)
	command.CreatedAt = time.Now().UTC().Add(2 * time.Minute).Truncate(time.Microsecond)
	if err := store.InsertCreation(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	token := uuid.New()
	claimed, err := store.ClaimCreation(t.Context(), token)
	if err != nil || claimed.ID != command.ID || claimed.LeaseToken != token {
		t.Fatal("app clock delayed accepted command", claimed, err)
	}
	// Even with the same token, the DB rejects expiry; no app-clock check is needed.
	if _, err := db.Exec(t.Context(), "UPDATE company_creation_commands SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", command.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteCreation(t.Context(), command.ID, uuid.Must(uuid.NewV7()), time.Now().UTC(), token); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatal("expired DB lease accepted", err)
	}
}
