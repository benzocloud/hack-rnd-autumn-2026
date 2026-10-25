package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type creationAccounts struct {
	accountOperations
	input  account.BeginCompanyCreationInput
	result account.CreationCommand
	err    error
}

func (a *creationAccounts) BeginCompanyCreation(_ context.Context, input account.BeginCompanyCreationInput) (account.CreationCommand, error) {
	a.input = input
	return a.result, a.err
}
func (a *creationAccounts) CompleteCompanyCreation(context.Context, account.CompleteCompanyCreationInput) (account.CreationCommand, error) {
	return a.result, a.err
}
func (a *creationAccounts) AuthorizeCompanyBootstrap(context.Context, account.AuthorizeCompanyBootstrapInput) (account.CreationCommand, error) {
	return a.result, a.err
}

func TestCompanyCreationAdaptersAndErrors(t *testing.T) {
	command := account.CreationCommand{ID: uuid.New(), CompanyID: uuid.Must(uuid.NewV7()), InitiatorID: uuid.New(), Spec: account.CompanySpec{Name: "Кофейня", Currency: "RUB", Timezone: "Europe/Moscow"}, State: "PREPARED", RequestSHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC()}
	accounts := &creationAccounts{result: command}
	server := New(accounts)
	write := &pb.WriteContext{Browser: &pb.BrowserContext{CookieToken: "opaque-cookie"}, CsrfToken: "opaque-csrf"}
	request := &pb.BeginCompanyCreationRequest{Context: write, IdempotencyKey: "company-key", Company: &pb.CompanyCreateSpec{Name: " Кофейня ", Currency: "RUB", Timezone: "Europe/Moscow"}}
	got, err := server.BeginCompanyCreation(t.Context(), request)
	if err != nil || got.GetCommand().GetId() != command.ID.String() || got.GetCommand().GetInitiatorId() != command.InitiatorID.String() || got.GetCommand().GetCompany().Inn != nil {
		t.Fatal(got, err)
	}
	if accounts.input.Context.CookieToken != "opaque-cookie" || accounts.input.Context.CSRFToken != "opaque-csrf" || accounts.input.Key != "company-key" || accounts.input.Spec.Name != " Кофейня " {
		t.Fatal(accounts.input)
	}
	for _, r := range []*pb.BeginCompanyCreationRequest{nil, {Company: &pb.CompanyCreateSpec{Inn: new(string)}}} {
		if _, err := server.BeginCompanyCreation(t.Context(), r); status.Code(err) != codes.InvalidArgument {
			t.Fatal("nil/empty spec accepted", err)
		}
	}
	for _, id := range []string{"", uuid.Nil.String(), command.CompanyID.String(), "URN:UUID:" + command.ID.String()} {
		if _, err := server.CompleteCompanyCreation(t.Context(), &pb.CompleteCompanyCreationRequest{Context: write, CommandId: id}); status.Code(err) != codes.InvalidArgument {
			t.Fatal("invalid command ID accepted", id, err)
		}
	}
	for _, tt := range []struct {
		name   string
		err    error
		code   codes.Code
		reason pb.AuthErrorReason
	}{
		{"conflict", account.ErrIdempotencyConflict, codes.Aborted, pb.AuthErrorReason_AUTH_ERROR_REASON_IDEMPOTENCY_CONFLICT},
		{"progress", account.ErrCreationInProgress, codes.Unavailable, pb.AuthErrorReason_AUTH_ERROR_REASON_COMPANY_CREATION_IN_PROGRESS},
		{"dependency", account.ErrUnavailable, codes.Unavailable, pb.AuthErrorReason_AUTH_ERROR_REASON_DEPENDENCY_UNAVAILABLE},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accounts.err = errors.Join(tt.err, errors.New("PRIVATE_DIAGNOSTIC"))
			_, err := server.CompleteCompanyCreation(t.Context(), &pb.CompleteCompanyCreationRequest{Context: write, CommandId: command.ID.String()})
			if status.Code(err) != tt.code || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC") {
				t.Fatal(err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatal(details)
			}
			detail, ok := details[0].(*pb.AuthErrorDetail)
			if !ok || detail.Reason != tt.reason {
				t.Fatal(details)
			}
		})
	}
}
