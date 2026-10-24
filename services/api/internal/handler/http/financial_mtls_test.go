package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/core"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

// This proves the HTTP/protobuf/mTLS adapter boundary, not Core job persistence.
type financialRPCFixture struct {
	corev1.UnimplementedCompanyServiceServer
	corev1.UnimplementedLedgerServiceServer
	corev1.UnimplementedForecastingServiceServer
	mu sync.Mutex
	f  *financialFixture
}

func (s *financialRPCFixture) ReadCompany(_ context.Context, _ *corev1.ReadCompanyRequest) (*corev1.ReadCompanyResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return proto.Clone(s.f.company).(*corev1.ReadCompanyResponse), nil
}

func (s *financialRPCFixture) GetDefaultAccount(_ context.Context, _ *corev1.GetDefaultAccountRequest) (*corev1.GetDefaultAccountResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &corev1.GetDefaultAccountResponse{Account: proto.Clone(s.f.account).(*corev1.Account)}, nil
}

func (s *financialRPCFixture) CreateForecast(_ context.Context, r *corev1.CreateForecastRequest) (*corev1.CreateForecastResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.f.create = proto.Clone(r).(*corev1.CreateForecastRequest)
	return &corev1.CreateForecastResponse{Accepted: proto.Clone(s.f.accepted).(*corev1.ForecastAccepted)}, nil
}

func (s *financialRPCFixture) GetForecast(_ context.Context, _ *corev1.GetForecastRequest) (*corev1.GetForecastResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &corev1.GetForecastResponse{Forecast: proto.Clone(s.f.forecast).(*corev1.Forecast)}, nil
}

func (s *financialRPCFixture) ListForecasts(_ context.Context, _ *corev1.ListForecastsRequest) (*corev1.ListForecastsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return proto.Clone(s.f.listed).(*corev1.ListForecastsResponse), nil
}

func TestFinancialHTTPThroughCoreMTLS(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := securetls.Load(p.CA, p.CoreCert, p.CoreKey, "spiffe://saldo.local/api", true)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &financialRPCFixture{f: newFinancialFixture()}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(identity)))
	corev1.RegisterCompanyServiceServer(server, fixture)
	corev1.RegisterLedgerServiceServer(server, fixture)
	corev1.RegisterForecastingServiceServer(server, fixture)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := coreclient.Dial(listener.Addr().String(), "localhost", p.CA, p.APICert, p.APIKey, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	h := NewWithCore(newFinancialFixture(), "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: corev1.NewCompanyServiceClient(conn), Ledger: corev1.NewLedgerServiceClient(conn), Forecasts: corev1.NewForecastingServiceClient(conn)})
	for _, path := range []string{"/api/v1/companies", financialRoot, financialRoot + "/account", financialRoot + "/forecasts"} {
		if w := financialRequest(h, "GET", path, ""); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	body := `{"cutoff_date":"2026-10-23","horizon_days":30}`
	for range 2 {
		w := financialRequest(h, "POST", financialRoot+"/forecasts", body)
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		var accepted map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil {
			t.Fatal(err)
		}
		if accepted["id"] != forecastID || accepted["status"] != "queued" || accepted["poll_after_ms"] != float64(1000) {
			t.Fatal(accepted)
		}
	}
	fixture.mu.Lock()
	request := proto.Clone(fixture.f.create).(*corev1.CreateForecastRequest)
	fixture.mu.Unlock()
	if request.Scope.CompanyId != companyID || request.Scope.AccountId != accountID || request.Context.Browser.CookieToken != "opaque-session" || request.Context.CsrfToken != strings.Repeat("c", 24) || request.IdempotencyKey != "financial-key-123" || request.HorizonDays != 30 {
		t.Fatal("transport changed scope/context/key", request)
	}
	w := financialRequest(h, "GET", financialRoot+"/forecasts/"+forecastID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"queued"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	fixture.mu.Lock()
	succeeded(fixture.f)
	fixture.mu.Unlock()
	first := financialRequest(h, "GET", financialRoot+"/forecasts/"+forecastID, "")
	second := financialRequest(h, "GET", financialRoot+"/forecasts/"+forecastID, "")
	if first.Code != 200 || second.Code != 200 || first.Body.String() != second.Body.String() || !strings.Contains(first.Body.String(), `"status":"succeeded"`) {
		t.Fatal(first.Code, first.Body.String(), second.Code)
	}
	server.Stop()
	if w := financialRequest(h, "GET", financialRoot+"/account", ""); w.Code != 503 {
		t.Fatal("Core outage not mapped safely", w.Code, w.Body.String())
	}
}
