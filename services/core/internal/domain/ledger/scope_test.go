package ledger_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestScopeRejectsNilUUID(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000001")
	for _, scope := range []ledger.Scope{{}, {CompanyID: id}, {AccountID: id}} {
		if !errors.Is(scope.Validate(), ledger.ErrScope) {
			t.Fatal("nil scope ID accepted")
		}
	}
	if err := (ledger.Scope{CompanyID: id, AccountID: id}).Validate(); err != nil {
		t.Fatal(err)
	}
}
