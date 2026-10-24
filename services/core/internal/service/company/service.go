package company

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type store interface {
	ReserveBootstrap(context.Context, domain.BootstrapRecord) (domain.BootstrapRecord, bool, error)
	InsertCompany(context.Context, uuid.UUID) error
	InsertAccount(context.Context, *ledger.BankAccount) error
	InsertProfile(context.Context, domain.Profile) error
	ReadProfile(context.Context, uuid.UUID) (domain.Profile, error)
	ReadBootstrap(context.Context, uuid.UUID) (domain.BootstrapRecord, error)
	UpdateProfile(context.Context, domain.Profile, int64) error
}

// Work borrows one transaction; app owns Begin/Commit/Rollback.
type Work func(context.Context, store) error

// Authorization contains the full durable Auth command, not a client boolean.
// The callback must call Auth AuthorizeCompanyBootstrap with a trusted Core peer.
type Authorization struct {
	CommandID, CompanyID uuid.UUID
	RequestSHA256        string
	Spec                 domain.Spec
}

type service struct {
	withinTx  func(context.Context, Work) error
	authorize func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error)
}

func New(withinTx func(context.Context, Work) error, authorize func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error)) *service {
	return &service{withinTx: withinTx, authorize: authorize}
}
