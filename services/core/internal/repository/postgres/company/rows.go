package company

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

type profileRow struct {
	CompanyID uuid.UUID `db:"company_id"`
	AccountID uuid.UUID `db:"account_id"`
	Name      string    `db:"name"`
	INN       string    `db:"inn"`
	Currency  string    `db:"currency"`
	Timezone  string    `db:"timezone"`
	Version   int64     `db:"version"`
	CreatedAt time.Time `db:"created_at"`
}

func (r profileRow) profile() (domain.Profile, error) {
	p := domain.Profile{ID: r.CompanyID, AccountID: r.AccountID, Spec: domain.Spec{Name: r.Name, INN: r.INN, Currency: r.Currency, Timezone: r.Timezone}, Version: r.Version, CreatedAt: r.CreatedAt.UTC()}
	if p.Validate() != nil {
		return domain.Profile{}, storage.ErrUnavailable
	}
	return p, nil
}

type bootstrapRow struct {
	CommandID     uuid.UUID `db:"command_id"`
	CompanyID     uuid.UUID `db:"company_id"`
	AccountID     uuid.UUID `db:"account_id"`
	RequestSHA256 string    `db:"request_sha256"`
	Name          string    `db:"name"`
	INN           string    `db:"inn"`
	Currency      string    `db:"currency"`
	Timezone      string    `db:"timezone"`
	CommittedAt   time.Time `db:"committed_at"`
}

func (r bootstrapRow) record() (domain.BootstrapRecord, error) {
	record := domain.BootstrapRecord{CommandID: r.CommandID, CompanyID: r.CompanyID, AccountID: r.AccountID, RequestSHA256: r.RequestSHA256, Spec: domain.Spec{Name: r.Name, INN: r.INN, Currency: r.Currency, Timezone: r.Timezone}, CommittedAt: r.CommittedAt.UTC()}
	if record.Validate() != nil {
		return domain.BootstrapRecord{}, storage.ErrUnavailable
	}
	return record, nil
}
