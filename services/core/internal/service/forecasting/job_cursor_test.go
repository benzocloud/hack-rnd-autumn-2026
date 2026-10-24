package forecasting

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestForecastCursorRejectsCrossActorScopeAndFilter(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	cursor := forecastCursor{Version: 1, Scope: scope, Actor: "owner", IncludeScenarios: false, Before: time.Now().UTC(), ID: uuid.New()}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	command := ListForecastCommand{Scope: scope, ActorID: "owner", Cursor: base64.RawURLEncoding.EncodeToString(encoded), Limit: 50}
	if _, err := decodeForecastCursor(command); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"actor", "account", "company", "filter", "malformed"} {
		changed := command
		switch mode {
		case "actor":
			changed.ActorID = "financier"
		case "account":
			changed.Scope.AccountID = uuid.New()
		case "company":
			changed.Scope.CompanyID = uuid.New()
		case "filter":
			changed.IncludeScenarios = true
		case "malformed":
			changed.Cursor = "invalid"
		}
		if _, err := decodeForecastCursor(changed); !errors.Is(err, ErrCursor) {
			t.Fatal(mode, err)
		}
	}
}
