//go:build integration

package app

import (
	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

// Explicit predictor double for tests whose subject is import/job transport.
// Actual categorization and money use Python/mTLS in the CSV lifecycle test.
func fixtureUnknownAnalysis(input forecasting.FinancialModelInput) *forecasting.Analysis {
	if input.Assessment == nil {
		return nil
	}
	a := &forecasting.Analysis{Version: forecasting.AnalysisVersion, From: input.Assessment.From, Through: input.Snapshot.Cutoff}
	for _, op := range input.Operations {
		d := op.Data()
		source, rule := "unknown", "no_matching_rule"
		if d.Category.ID != uuid.Nil {
			source, rule = "manual", ""
		}
		a.Operations = append(a.Operations, forecasting.OperationAnalysis{ID: d.ID, Version: op.Version(), CategoryCode: "unknown", CategorySource: source, RuleID: rule, CategoryID: d.Category.ID})
	}
	return a
}
