package forecasting

import (
	"encoding/json"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

const financialJobVersion = "mean_90_components_v1"
const financialJobPreprocessing = "calendar_replacement_v1"
const forecastJobOperation = "create_forecast_v1"

type jobInput struct {
	SchemaVersion              int
	SnapshotID, ModelContextID uuid.UUID
	Scope                      ledger.Scope
	DataVersion                int64
	Cutoff                     ledger.Date
	HorizonDays                int
	Coverage                   domain.JobCoverage
	Model                      ModelContext
}

func frozenInput(snapshot domain.FinancialSnapshot, coverage domain.JobCoverage) (jobInput, error) {
	d := snapshot.Data()
	id, err := uuid.NewV7()
	if err != nil {
		return jobInput{}, ErrUnavailable
	}
	return jobInput{SchemaVersion: 1, SnapshotID: d.ID, ModelContextID: id, Scope: d.Scope, DataVersion: d.DataVersion, Cutoff: d.Cutoff, HorizonDays: 30, Coverage: coverage, Model: ModelContext{Name: "mean_90", Version: financialJobVersion, PreprocessingVersion: financialJobPreprocessing}}, nil
}

func decodeJobInput(job domain.Job) (jobInput, error) {
	var input jobInput
	if len(job.Input) == 0 || len(job.Input) > 2<<20 || json.Unmarshal(job.Input, &input) != nil || input.SchemaVersion != 1 || input.SnapshotID == uuid.Nil || input.ModelContextID == uuid.Nil || input.Scope != job.Scope || input.DataVersion != job.DataVersion || input.HorizonDays != 30 || input.Cutoff.Validate() != nil ||
		input.Model != (ModelContext{Name: "mean_90", Version: financialJobVersion, PreprocessingVersion: financialJobPreprocessing}) || !validCoverage(input.Coverage, input.DataVersion) {
		return jobInput{}, ErrUnavailable
	}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Cutoff))
	if input.Coverage.From > ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly)) || input.Coverage.Through < input.Cutoff {
		return jobInput{}, ErrUnavailable
	}
	return input, nil
}

func prepareFinancial(snapshot domain.FinancialSnapshot, proof domain.JobCoverage) (FinancialModelInput, error) {
	d := snapshot.Data()
	cutoff, _ := time.Parse(time.DateOnly, string(d.Cutoff))
	start := cutoff.AddDate(0, 0, -89)
	if start.Year() < 1 || cutoff.AddDate(0, 0, 30).Year() > 9999 || proof.From > ledger.Date(start.Format(time.DateOnly)) || proof.Through < d.Cutoff || !validCoverage(proof, d.DataVersion) {
		return FinancialModelInput{}, ErrHistory
	}
	input := FinancialModelInput{Snapshot: ModelInput{SnapshotID: d.ID, Scope: d.Scope, DataVersion: d.DataVersion, Cutoff: d.Cutoff, OpeningBalance: d.Opening.Observation.Amount(), HorizonDays: 30, Model: ModelContext{Name: "mean_90", Version: financialJobVersion, PreprocessingVersion: financialJobPreprocessing}}, Coverage: Coverage{Kind: proof.Kind, From: ledger.Date(start.Format(time.DateOnly)), Through: d.Cutoff, Reference: proof.Reference, DataVersion: proof.DataVersion, Limitations: slices.Clone(proof.Limitations)}}
	flows := make(map[ledger.Date][2]*big.Int)
	for _, fact := range d.Facts {
		op := fact.Transaction.Data()
		if op.Date < input.Coverage.From {
			continue
		}
		if len(input.Operations) >= 20000 {
			return FinancialModelInput{}, ErrHistory
		}
		input.Operations = append(input.Operations, fact.Transaction)
		sum, ok := flows[op.Date]
		if !ok {
			sum = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		n := 0
		if op.Direction == ledger.Outflow {
			n = 1
		}
		sum[n].Add(sum[n], big.NewInt(op.Amount.Minor()))
		flows[op.Date] = sum
	}
	input.Snapshot.History = make([]ModelDay, 90)
	balance := big.NewInt(d.Opening.Observation.Amount().Minor())
	for n := 89; n >= 0; n-- {
		date := ledger.Date(start.AddDate(0, 0, n).Format(time.DateOnly))
		sum, ok := flows[date]
		if !ok {
			sum = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		if !sum[0].IsInt64() || !sum[1].IsInt64() || !balance.IsInt64() {
			return FinancialModelInput{}, ErrHistory
		}
		incoming, _ := money.New(sum[0].Int64(), money.RUB)
		outgoing, _ := money.New(sum[1].Int64(), money.RUB)
		closing, _ := money.New(balance.Int64(), money.RUB)
		source := ReconstructedCovered
		if n == 89 {
			source = Observed
		}
		input.Snapshot.History[n] = ModelDay{Date: date, Inflow: incoming, Outflow: outgoing, ClosingBalance: closing, Source: source}
		balance.Sub(balance, sum[0])
		balance.Add(balance, sum[1])
	}
	return input, nil
}

func validCoverage(proof domain.JobCoverage, revision int64) bool {
	if (proof.Kind != "certified" && proof.Kind != "dataset_fixture_assumption") || proof.From.Validate() != nil || proof.Through.Validate() != nil || strings.TrimSpace(proof.Reference) == "" || len(proof.Reference) > 2048 || proof.DataVersion != revision || len(proof.Limitations) > 32 {
		return false
	}
	seen := make(map[string]bool, len(proof.Limitations))
	for _, limitation := range proof.Limitations {
		if strings.TrimSpace(limitation) == "" || len(limitation) > 128 || seen[limitation] {
			return false
		}
		seen[limitation] = true
	}
	return proof.Kind != "dataset_fixture_assumption" || slices.Contains(proof.Limitations, "dataset_coverage_assumed_not_bank_certified")
}
