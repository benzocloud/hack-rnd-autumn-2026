package payments

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func TestPaymentRPCRejectsInternalMatchVersionFallback(t *testing.T) {
	t.Parallel()
	h := &handler{}
	_, err := h.MatchPayment(t.Context(), &pb.MatchPaymentRequest{Context: &authpb.WriteContext{}, IdempotencyKey: "valid-key", ExpectedTransactionVersion: 0})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("version fallback code=%v", status.Code(err))
	}
}
func TestPaymentRPCCreateValidatesKeyAndDateBeforeDependencies(t *testing.T) {
	t.Parallel()
	for _, r := range []*pb.CreateManualPaymentRequest{
		{Context: &authpb.WriteContext{}, IdempotencyKey: "bad", ScheduledDate: "2026-10-25"},
		{Context: &authpb.WriteContext{}, IdempotencyKey: "valid-key", ScheduledDate: "2026-02-30"},
	} {
		_, err := (&handler{}).CreateManualPayment(t.Context(), r)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	}
}
func TestPaymentErrorMapsDomainValidation(t *testing.T) {
	t.Parallel()
	if status.Code(paymentError(ledger.ErrInvalid)) != codes.InvalidArgument {
		t.Fatal("domain validation became internal error")
	}
}
