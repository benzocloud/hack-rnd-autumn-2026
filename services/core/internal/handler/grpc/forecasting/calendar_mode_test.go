package forecasting

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func TestForecastRejectsUnsupportedCalendarSelection(t *testing.T) {
	h := &handler{}
	for _, tc := range []struct {
		mode pb.ForecastCalendarMode
		code codes.Code
	}{
		{pb.ForecastCalendarMode_FORECAST_CALENDAR_MODE_SYNTHETIC_SCENARIO, codes.FailedPrecondition},
		{pb.ForecastCalendarMode(99), codes.InvalidArgument},
	} {
		_, err := h.CreateForecast(t.Context(), &pb.CreateForecastRequest{Context: &authpb.WriteContext{}, CalendarMode: tc.mode})
		if status.Code(err) != tc.code {
			t.Fatal(tc.mode, err)
		}
	}
}
