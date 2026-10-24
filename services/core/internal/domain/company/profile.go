package company

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type Profile struct {
	ID, AccountID uuid.UUID
	Spec          Spec
	Version       int64
	CreatedAt     time.Time
}

func (p Profile) Update(spec Spec) (Profile, error) {
	if p.Validate() != nil || spec.Validate() != nil || p.Version == math.MaxInt64 {
		return Profile{}, ErrInvalid
	}
	p.Spec = spec
	p.Version++
	return p, nil
}

func NewProfile(id, accountID uuid.UUID, spec Spec, at time.Time) (Profile, error) {
	p := Profile{ID: id, AccountID: accountID, Spec: spec, Version: 1, CreatedAt: at}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	p.CreatedAt = p.CreatedAt.UTC().Truncate(time.Microsecond)
	return p, nil
}
func (p Profile) Validate() error {
	if p.ID == uuid.Nil || p.AccountID == uuid.Nil || p.ID == p.AccountID || p.Version < 1 || p.CreatedAt.IsZero() {
		return ErrInvalid
	}
	return p.Spec.Validate()
}
