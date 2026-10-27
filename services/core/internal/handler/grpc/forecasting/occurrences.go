package forecasting

import (
	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func encodeOccurrences(out *pb.ForecastResult, view forecasting.ForecastView) {
	metadata := &pb.ForecastResult{}
	encodeKnownComponents(metadata, view.Result, view.Snapshot.Data().Calendar)
	sources := map[string]*pb.ForecastComponent{}
	for _, c := range metadata.Components {
		sources[c.GetPaymentId()] = c
	}
	out.Recommendations = append(out.Recommendations, metadata.Recommendations...)
	for _, c := range view.Occurrences {
		direction := pb.Direction_DIRECTION_INFLOW
		if c.Direction == ledger.Outflow {
			direction = pb.Direction_DIRECTION_OUTFLOW
		}
		component := &pb.ForecastComponent{Id: c.ComponentID, OccurrenceId: c.OccurrenceID, Kind: c.Kind, Direction: direction, Date: string(c.Date), Amount: kopecks(c.AmountMinor), RemainingAmount: kopecks(c.RemainingMinor)}
		occurrence := &pb.EffectiveOccurrence{EffectiveOccurrenceId: c.ID, ComponentId: c.ComponentID, OccurrenceId: c.OccurrenceID, Direction: direction, Date: string(c.Date), RemainingAmount: kopecks(c.RemainingMinor), InHorizon: c.Date >= view.Result.Days[0].Date && c.Date <= view.Result.Days[29].Date}
		occurrence.ResponseActionEligible = c.Direction == ledger.Outflow && occurrence.InHorizon && c.RemainingMinor > 0
		if c.PaymentID != uuid.Nil {
			component.PaymentId = new(c.PaymentID.String())
			occurrence.PaymentId = new(c.PaymentID.String())
			if source := sources[c.PaymentID.String()]; source != nil {
				component.Source = source.Source
				component.RecurrenceGroupId = source.RecurrenceGroupId
			}
		}
		if c.CategoryID != uuid.Nil {
			component.CategoryId = new(c.CategoryID.String())
		}
		out.Components = append(out.Components, component)
		out.EffectiveOccurrences = append(out.EffectiveOccurrences, occurrence)
	}
	out.ScenarioCapabilities = &pb.ScenarioCapabilities{UnavailableReason: new("Новый стресс создаётся от исходного прогноза; здесь доступен перенос части расхода.")}
}
