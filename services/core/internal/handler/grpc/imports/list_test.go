package imports

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

type listFixture struct {
	operations
	scope ledger.Scope
	items []domain.Record
	calls []service.ListCommand
}

func (f *listFixture) List(_ context.Context, cmd service.ListCommand) (service.ListResult, error) {
	f.calls = append(f.calls, cmd)
	start := 0
	if cmd.Cursor != "" {
		var err error
		start, err = strconv.Atoi(cmd.Cursor)
		if err != nil {
			return service.ListResult{}, domain.ErrCursor
		}
	}
	end := min(len(f.items), start+cmd.Limit)
	cursor := ""
	if end < len(f.items) {
		cursor = strconv.Itoa(end)
	}
	return service.ListResult{Items: f.items[start:end], Cursor: cursor}, nil
}
func (f *listFixture) ReadProfile(context.Context, uuid.UUID) (company.Profile, error) {
	return company.Profile{ID: f.scope.CompanyID, AccountID: f.scope.AccountID}, nil
}
func (f *listFixture) AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	return &authpb.AuthorizeCompanyResponse{ActorId: "reader"}, nil
}

func TestListImportsShrinksPageWithoutSkippingRecords(t *testing.T) {
	f := &listFixture{scope: ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}}
	balances := make([]domain.Balance, 1000)
	for i := range balances {
		balances[i] = domain.Balance{Date: fmt.Sprintf("2026-%02d-%02d", 1+i/28%12, 1+i%28), Minor: 9223372036854775807}
	}
	for range 100 {
		f.items = append(f.items, domain.Record{ID: uuid.New(), Metadata: domain.Metadata{Scope: f.scope, Adapter: "dataset_v1", Currency: "RUB", SourceCompanyID: "C001"}, Status: "preview_ready", Version: 2, CreatedAt: time.Now(), UpdatedAt: time.Now(), Preview: &domain.Preview{Balances: balances, BaseVersion: 1}})
	}
	h := &handler{imports: f, companies: f, auth: f}
	limit := uint32(100)
	request := &pb.ListImportsRequest{Scope: &pb.AccountScope{CompanyId: f.scope.CompanyID.String(), AccountId: f.scope.AccountID.String()}, Page: &pb.PageRequest{Limit: &limit}}
	seen := make(map[string]bool)
	for {
		before := len(f.calls)
		response, err := h.ListImports(t.Context(), request)
		if err != nil || proto.Size(response) > importListBudget {
			t.Fatal("unbounded response", err)
		}
		if len(f.calls)-before > 7 || len(response.Items) == 0 {
			t.Fatal("unbounded retries or empty page")
		}
		for _, item := range response.Items {
			if seen[item.Id] {
				t.Fatal("duplicate record")
			}
			seen[item.Id] = true
		}
		if response.Pagination.NextCursor == "" {
			break
		}
		if response.Pagination.NextCursor != strconv.Itoa(len(seen)) {
			t.Fatal("cursor skipped omitted records", response.Pagination.NextCursor, len(seen))
		}
		request.Page.Cursor = response.Pagination.NextCursor
	}
	if len(seen) != 100 || len(f.calls) < 2 || f.calls[1].Limit >= f.calls[0].Limit {
		t.Fatal("missing records or no retry", len(seen), f.calls)
	}
}

func TestListImportsRejectsSingleOversizedResource(t *testing.T) {
	f := &listFixture{scope: ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}}
	f.items = []domain.Record{{ID: uuid.New(), Status: "preview_ready", Preview: &domain.Preview{Issues: []domain.Issue{{ExternalIDs: []string{strings.Repeat("x", 2<<20)}}}}}}
	h := &handler{imports: f, companies: f, auth: f}
	_, err := h.ListImports(t.Context(), &pb.ListImportsRequest{Scope: &pb.AccountScope{CompanyId: f.scope.CompanyID.String(), AccountId: f.scope.AccountID.String()}})
	if status.Code(err) != codes.ResourceExhausted || strings.Contains(err.Error(), "xxx") || len(f.calls) > 7 {
		t.Fatal("single oversized resource not rejected safely", err, len(f.calls))
	}
}
