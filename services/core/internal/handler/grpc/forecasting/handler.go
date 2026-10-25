package forecasting

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	companyDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type operations interface {
	Create(context.Context, forecasting.CreateForecastCommand) (forecasting.ForecastAccepted, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (forecasting.ForecastView, error)
	List(context.Context, forecasting.ListForecastCommand) (forecasting.ListForecastResult, error)
}
type companies interface {
	ReadProfile(context.Context, uuid.UUID) (companyDomain.Profile, error)
}
type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}
type handler struct {
	pb.UnimplementedForecastingServiceServer
	pb.UnimplementedScenariosServiceServer
	scenarios scenarioOperations
	jobs      operations
	companies companies
	auth      authorizer
	location  *time.Location
	now       func() time.Time
}

func Register(server grpc.ServiceRegistrar, jobs operations, companies companies, auth authorizer, scenarios ...scenarioOperations) error {
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return forecasting.ErrUnavailable
	}
	h := &handler{jobs: jobs, companies: companies, auth: auth, location: location, now: time.Now}
	if len(scenarios) > 0 {
		h.scenarios = scenarios[0]
		pb.RegisterScenariosServiceServer(server, h)
	}
	pb.RegisterForecastingServiceServer(server, h)
	return nil
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		return uuid.Nil, forecasting.ErrInvalid
	}
	return id, nil
}
func parseScope(raw *pb.AccountScope) (ledger.Scope, error) {
	if raw == nil {
		return ledger.Scope{}, forecasting.ErrInvalid
	}
	company, err := parseID(raw.CompanyId)
	if err != nil {
		return ledger.Scope{}, err
	}
	account, err := parseID(raw.AccountId)
	if err != nil {
		return ledger.Scope{}, err
	}
	return ledger.Scope{CompanyID: company, AccountID: account}, nil
}

func (h *handler) authorize(ctx context.Context, scope ledger.Scope, browser *authpb.BrowserContext, write *authpb.WriteContext) (string, error) {
	request := &authpb.AuthorizeCompanyRequest{CompanyId: scope.CompanyID.String(), Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ, Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: browser}}
	if write != nil {
		request.Permission = authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE
		request.Context = &authpb.AuthorizeCompanyRequest_Write{Write: write}
	}
	grant, err := h.auth.AuthorizeCompany(ctx, request)
	if err != nil {
		return "", err
	}
	if grant == nil || grant.ActorId == "" {
		return "", forecasting.ErrUnavailable
	}
	profile, err := h.companies.ReadProfile(ctx, scope.CompanyID)
	if err != nil {
		return "", err
	}
	if profile.AccountID != scope.AccountID {
		return "", forecasting.ErrNotFound
	}
	return grant.ActorId, nil
}

func (h *handler) CreateForecast(ctx context.Context, r *pb.CreateForecastRequest) (*pb.CreateForecastResponse, error) {
	if r == nil || r.Context == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	switch r.CalendarMode {
	case pb.ForecastCalendarMode_FORECAST_CALENDAR_MODE_UNSPECIFIED, pb.ForecastCalendarMode_FORECAST_CALENDAR_MODE_AVAILABLE:
	case pb.ForecastCalendarMode_FORECAST_CALENDAR_MODE_SYNTHETIC_SCENARIO:
		return nil, rpcerror.From(forecasting.ErrHorizon)
	default:
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	actor, err := h.authorize(ctx, scope, nil, r.Context)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	result, err := h.jobs.Create(ctx, forecasting.CreateForecastCommand{Scope: scope, ActorID: actor, IdempotencyKey: r.IdempotencyKey, Cutoff: ledger.Date(r.CutoffDate), Today: ledger.Date(h.now().In(h.location).Format(time.DateOnly)), HorizonDays: int(r.HorizonDays)})
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.CreateForecastResponse{Accepted: &pb.ForecastAccepted{Id: result.ID.String(), Status: pb.ForecastStatus_FORECAST_STATUS_QUEUED, PollAfterMs: result.PollAfterMS}}, nil
}

func (h *handler) GetForecast(ctx context.Context, r *pb.GetForecastRequest) (*pb.GetForecastResponse, error) {
	if r == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	id, err := parseID(r.ForecastId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if _, err := h.authorize(ctx, scope, r.Context, nil); err != nil {
		return nil, rpcerror.From(err)
	}
	result, err := h.readForecast(ctx, scope, id)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.GetForecastResponse{Forecast: encodeForecast(result)}, nil
}

func (h *handler) ListForecasts(ctx context.Context, r *pb.ListForecastsRequest) (*pb.ListForecastsResponse, error) {
	if r == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	actor, err := h.authorize(ctx, scope, r.Context, nil)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	limit := 50
	cursor := ""
	if r.Page != nil {
		cursor = r.Page.Cursor
		if r.Page.Limit != nil {
			if *r.Page.Limit == 0 || *r.Page.Limit > 100 {
				return nil, rpcerror.From(forecasting.ErrInvalid)
			}
			limit = int(*r.Page.Limit)
		}
	}
	result, err := h.jobs.List(ctx, forecasting.ListForecastCommand{Scope: scope, ActorID: actor, Cursor: cursor, Limit: limit, IncludeScenarios: r.IncludeScenarios})
	if err != nil {
		return nil, rpcerror.From(err)
	}
	response := &pb.ListForecastsResponse{Items: []*pb.ForecastSummary{}, Pagination: &pb.PageResponse{NextCursor: result.NextCursor}}
	for _, item := range result.Items {
		response.Items = append(response.Items, encodeSummary(item))
	}
	return response, nil
}

func (h *handler) readForecast(ctx context.Context, scope ledger.Scope, id uuid.UUID) (forecasting.ForecastView, error) {
	v, err := h.jobs.Read(ctx, scope, id)
	if errors.Is(err, forecasting.ErrNotFound) && h.scenarios != nil {
		return h.scenarios.Forecast(ctx, scope, id)
	}
	return v, err
}
