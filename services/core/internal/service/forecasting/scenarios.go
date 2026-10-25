package forecasting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type scenarioStore interface {
	Insert(context.Context, domain.Scenario) error
	Read(context.Context, ledger.Scope, uuid.UUID, bool) (domain.Scenario, error)
	List(context.Context, ledger.Scope, uuid.UUID, uuid.UUID, int) ([]domain.Scenario, error)
}
type ScenarioWork func(context.Context, scenarioStore, jobCommands) error
type scenarioBase interface {
	Read(context.Context, ledger.Scope, uuid.UUID) (ForecastView, error)
}
type scenarios struct {
	work func(context.Context, ScenarioWork) error
	base scenarioBase
}

func NewScenarios(work func(context.Context, ScenarioWork) error, base scenarioBase) *scenarios {
	return &scenarios{work: work, base: base}
}

type ExpenseChange struct {
	ID          uuid.UUID
	Date        ledger.Date
	AmountMinor int64
	Description string
}
type ScenarioCommand struct {
	Scope                         ledger.Scope
	ActorID, IdempotencyKey, Name string
	BaseForecastID                uuid.UUID
	Changes                       []ExpenseChange
	SalesChanges                  []SalesReductionChange
	DelayChanges                  []DelayReceiptChange
	ParentScenarioID              uuid.UUID
	Response                      *ResponseAction
}
type ScenarioView struct {
	Record           domain.Scenario
	Changes          []ExpenseChange
	SalesChanges     []SalesReductionChange
	DelayChanges     []DelayReceiptChange
	ParentScenarioID *uuid.UUID
	Response         *ResponseAction
	OutOfHorizonIDs  []string
}
type ScenarioListCommand struct {
	Scope           ledger.Scope
	ActorID, Cursor string
	BaseForecastID  uuid.UUID
	Limit           int
}
type ScenarioList struct {
	Items      []ScenarioView
	NextCursor string
}
type scenarioPayload struct {
	SchemaVersion    int
	Changes          []ExpenseChange
	SalesChanges     []SalesReductionChange `json:",omitempty"`
	SalesBasis       []SalesComponent       `json:",omitempty"`
	DelayChanges     []DelayReceiptChange   `json:",omitempty"`
	ParentScenarioID *uuid.UUID             `json:",omitempty"`
	ResponseAction   *ResponseAction        `json:",omitempty"`
	Occurrences      []EffectiveOccurrence  `json:",omitempty"`
	OutOfHorizonIDs  []string               `json:",omitempty"`
	Result           SavedForecast
}

func validateScenario(cmd ScenarioCommand) error {
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || !validForecastKey(cmd.IdempotencyKey) || strings.TrimSpace(cmd.Name) == "" || !utf8.ValidString(cmd.Name) || utf8.RuneCountInString(cmd.Name) > 200 {
		return ErrInvalid
	}
	count := len(cmd.Changes) + len(cmd.SalesChanges) + len(cmd.DelayChanges)
	if cmd.ParentScenarioID != uuid.Nil {
		if cmd.BaseForecastID != uuid.Nil || count != 0 || validateResponse(cmd.Response) != nil {
			return ErrInvalid
		}
		return nil
	}
	if cmd.Response != nil || cmd.BaseForecastID == uuid.Nil || count < 1 || count > 20 {
		return ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, c := range cmd.Changes {
		if c.ID == uuid.Nil || seen[c.ID] || c.Date.Validate() != nil || c.AmountMinor <= 0 || strings.TrimSpace(c.Description) == "" || !utf8.ValidString(c.Description) || utf8.RuneCountInString(c.Description) > 1000 {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	for _, c := range cmd.SalesChanges {
		if validateSalesChange(c) != nil || seen[c.ID] {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	for _, c := range cmd.DelayChanges {
		if validateDelay(c) != nil || seen[c.ID] {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	return nil
}

// ApplyScenario only adds explicit expenses to an immutable saved projection.
// Money.Add rejects overflow; neither input slices nor financial facts mutate.
func ApplyScenario(base SavedForecast, changes []ExpenseChange) (SavedForecast, error) {
	if len(base.Days) != 30 {
		return SavedForecast{}, ErrInvalid
	}
	encoded, err := json.Marshal(base)
	if err != nil {
		return SavedForecast{}, ErrUnavailable
	}
	var out SavedForecast
	if json.Unmarshal(encoded, &out) != nil {
		return out, ErrUnavailable
	}
	for _, c := range changes {
		if c.AmountMinor <= 0 || c.Date < out.Days[0].Date || c.Date > out.Days[29].Date {
			return SavedForecast{}, ErrInvalid
		}
		found := false
		for i := range out.Days {
			d := &out.Days[i]
			if d.Date < c.Date {
				continue
			}
			if d.Date == c.Date {
				if d.Outflow > math.MaxInt64-c.AmountMinor {
					return SavedForecast{}, ErrInvalid
				}
				d.Outflow += c.AmountMinor
				found = true
			}
			balance, _ := money.New(d.Balance, money.RUB)
			expense, _ := money.New(-c.AmountMinor, money.RUB)
			next, e := balance.Add(expense)
			if e != nil || next.Minor() == math.MinInt64 {
				return SavedForecast{}, ErrInvalid
			}
			d.Balance = next.Minor()
		}
		if !found {
			return SavedForecast{}, ErrInvalid
		}
	}
	out.FirstNegativeDate = nil
	for _, d := range out.Days {
		if d.Balance < 0 {
			v := d.Date
			out.FirstNegativeDate = &v
			break
		}
	}
	if len(changes) > 0 {
		out.Assumptions = append(out.Assumptions, "scenario_one_off_expense_v1: explicit additional expenses; original forecast and financial facts unchanged")
	}
	return out, nil
}

func (s *scenarios) Create(ctx context.Context, cmd ScenarioCommand) (ScenarioView, error) {
	if err := validateScenario(cmd); err != nil {
		return ScenarioView{}, err
	}
	prepared, baseID, err := s.prepareScenario(ctx, cmd)
	if err != nil {
		return ScenarioView{}, err
	}
	var parent *uuid.UUID
	if cmd.ParentScenarioID != uuid.Nil {
		parent = &cmd.ParentScenarioID
	}
	body, _ := json.Marshal(struct {
		Name     string
		Base     uuid.UUID
		Changes  []ExpenseChange
		Sales    []SalesReductionChange `json:",omitempty"`
		Delays   []DelayReceiptChange   `json:",omitempty"`
		Parent   *uuid.UUID             `json:",omitempty"`
		Response *ResponseAction        `json:",omitempty"`
	}{cmd.Name, cmd.BaseForecastID, cmd.Changes, cmd.SalesChanges, cmd.DelayChanges, parent, cmd.Response})

	digest := sha256.Sum256(body)
	var record domain.Scenario
	err = s.work(ctx, func(ctx context.Context, store scenarioStore, commands jobCommands) error {
		receipt, replay, e := commands.Reserve(ctx, cmd.Scope, cmd.ActorID, "scenario.create.v1", "", cmd.IdempotencyKey, digest[:])
		if e != nil {
			return e
		}
		if replay {
			var id uuid.UUID
			if json.Unmarshal(receipt, &id) != nil || id == uuid.Nil {
				return ErrUnavailable
			}
			record, e = store.Read(ctx, cmd.Scope, id, false)
			return e
		}
		id, e := uuid.NewV7()
		if e != nil {
			return ErrUnavailable
		}
		fid, e := uuid.NewV7()
		if e != nil {
			return ErrUnavailable
		}
		payload, e := json.Marshal(prepared)
		if e != nil || len(payload) > 2<<20 {
			return ErrUnavailable
		}
		record = domain.Scenario{Scope: cmd.Scope, ID: id, ForecastID: fid, BaseForecastID: baseID, Name: cmd.Name, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), Payload: payload}
		if e := store.Insert(ctx, record); e != nil {
			return e
		}
		receipt, e = json.Marshal(id)
		if e != nil {
			return ErrUnavailable
		}
		return commands.Complete(ctx, cmd.Scope, cmd.ActorID, "scenario.create.v1", "", cmd.IdempotencyKey, receipt)
	})
	if err != nil {
		return ScenarioView{}, err
	}
	return scenarioView(record)
}
func scenarioView(record domain.Scenario) (ScenarioView, error) {
	var p scenarioPayload
	if json.Unmarshal(record.Payload, &p) != nil || (p.SchemaVersion < 1 || p.SchemaVersion > 3) || len(p.Changes)+len(p.SalesChanges)+len(p.DelayChanges) < 1 || len(p.Changes)+len(p.SalesChanges)+len(p.DelayChanges) > 20 {
		return ScenarioView{}, ErrUnavailable
	}
	if (p.ParentScenarioID == nil) != (p.ResponseAction == nil) {
		return ScenarioView{}, ErrUnavailable
	}
	return ScenarioView{Record: record, Changes: p.Changes, SalesChanges: p.SalesChanges, DelayChanges: p.DelayChanges, ParentScenarioID: p.ParentScenarioID, Response: p.ResponseAction, OutOfHorizonIDs: p.OutOfHorizonIDs}, nil
}
func (s *scenarios) Read(ctx context.Context, scope ledger.Scope, id uuid.UUID) (ScenarioView, error) {
	if scope.Validate() != nil || id == uuid.Nil {
		return ScenarioView{}, ErrInvalid
	}
	var r domain.Scenario
	err := s.work(ctx, func(ctx context.Context, store scenarioStore, _ jobCommands) error {
		var e error
		r, e = store.Read(ctx, scope, id, false)
		return e
	})
	if err != nil {
		return ScenarioView{}, err
	}
	return scenarioView(r)
}
func (s *scenarios) Forecast(ctx context.Context, scope ledger.Scope, id uuid.UUID) (ForecastView, error) {
	var r domain.Scenario
	err := s.work(ctx, func(ctx context.Context, store scenarioStore, _ jobCommands) error {
		var e error
		r, e = store.Read(ctx, scope, id, true)
		return e
	})
	if err != nil {
		return ForecastView{}, err
	}
	view, err := s.base.Read(ctx, scope, r.BaseForecastID)
	if err != nil {
		return ForecastView{}, err
	}
	var p scenarioPayload
	if json.Unmarshal(r.Payload, &p) != nil || (p.SchemaVersion < 1 || p.SchemaVersion > 3) || len(p.Result.Days) != 30 {
		return ForecastView{}, ErrUnavailable
	}
	view.ID = r.ForecastID
	view.ScenarioID = &r.ID
	view.ScenarioExpenses = p.Changes
	if p.SalesBasis != nil {
		view.Sales = p.SalesBasis
	}
	view.Occurrences = p.Occurrences
	if view.Occurrences == nil {
		view.Result = &p.Result
		view.Occurrences, err = ScenarioOccurrences(view)
		if err != nil {
			return ForecastView{}, err
		}
	}

	view.CreatedAt = r.CreatedAt
	view.CompletedAt = &r.CreatedAt
	view.Result = &p.Result
	return view, nil
}

type scenarioCursor struct {
	Scope        ledger.Scope
	Actor        string
	Base, Before uuid.UUID
}

func (s *scenarios) List(ctx context.Context, cmd ScenarioListCommand) (ScenarioList, error) {
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.Limit < 0 || cmd.Limit > 100 {
		return ScenarioList{}, ErrInvalid
	}
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	cursor := scenarioCursor{Scope: cmd.Scope, Actor: cmd.ActorID, Base: cmd.BaseForecastID}
	if cmd.Cursor != "" {
		if len(cmd.Cursor) > 1024 {
			return ScenarioList{}, ErrCursor
		}
		b, e := base64.RawURLEncoding.DecodeString(cmd.Cursor)
		if e != nil || json.Unmarshal(b, &cursor) != nil || cursor.Scope != cmd.Scope || cursor.Actor != cmd.ActorID || cursor.Base != cmd.BaseForecastID || cursor.Before == uuid.Nil {
			return ScenarioList{}, ErrCursor
		}
	}
	out := ScenarioList{Items: []ScenarioView{}}
	err := s.work(ctx, func(ctx context.Context, store scenarioStore, _ jobCommands) error {
		rows, e := store.List(ctx, cmd.Scope, cmd.BaseForecastID, cursor.Before, cmd.Limit+1)
		if e != nil {
			return e
		}
		more := len(rows) > cmd.Limit
		if more {
			rows = rows[:cmd.Limit]
		}
		for _, r := range rows {
			v, e := scenarioView(r)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if more {
			cursor.Before = rows[len(rows)-1].ID
			b, e := json.Marshal(cursor)
			if e != nil {
				return ErrUnavailable
			}
			out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
		}
		return nil
	})
	return out, err
}
