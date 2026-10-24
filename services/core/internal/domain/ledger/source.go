package ledger

import (
	"time"
)

type SourceKind string

const (
	Manual   SourceKind = "manual"
	Imported SourceKind = "imported"
)

type Namespace struct {
	System    string
	CompanyID string
}

func (n Namespace) Validate() error {
	if n.System == "" || n.CompanyID == "" {
		return ErrInvalid
	}
	return nil
}

type Source struct {
	Kind       SourceKind
	Namespace  Namespace
	ExternalID string
	ImportedAt time.Time
}

func (s Source) Validate() error {
	switch s.Kind {
	case Manual:
		if s.Namespace != (Namespace{}) || s.ExternalID != "" || !s.ImportedAt.IsZero() {
			return ErrInvalid
		}
	case Imported:
		if err := s.Namespace.Validate(); err != nil {
			return err
		}
		if s.ExternalID == "" || s.ImportedAt.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
