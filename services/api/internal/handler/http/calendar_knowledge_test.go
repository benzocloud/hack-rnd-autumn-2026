package httpapi

import (
	"testing"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestCalendarKnowledgeProjectionDoesNotInventHistory(t *testing.T) {
	i := &corev1.ForecastInput{CutoffDate: "2026-10-24", StartDate: "2026-10-25", ModelContext: &corev1.ForecastModelContext{ModelVersion: "mean_90_components_v1"}}
	if !validCalendarKnowledge(i) || calendarKnowledgeName(i.CalendarKnowledgeMode) != nil {
		t.Fatal("legacy mode must stay unknown")
	}
	i.CalendarKnowledgeMode = corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_CURRENT
	i.CalendarAsOfDate = i.StartDate
	if validCalendarKnowledge(i) {
		t.Fatal("next-day calendar needs factual bridge guard")
	}
	i.NoPostCutoffMovements = true
	if !validCalendarKnowledge(i) || calendarKnowledgeName(i.CalendarKnowledgeMode) != "current" {
		t.Fatal("valid current calendar rejected")
	}
	i.ModelContext.ModelVersion = "mean_90_csv_v1"
	if validCalendarKnowledge(i) {
		t.Fatal("CSV calendar capability must fail explicitly")
	}
	i.CalendarKnowledgeMode = corev1.ForecastCalendarKnowledgeMode(99)
	if validCalendarKnowledge(i) {
		t.Fatal("unknown knowledge mode")
	}
}
