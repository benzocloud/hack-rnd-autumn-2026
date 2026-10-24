package rpcerror

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestFinancialWriteErrorCodes(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   codes.Code
		reason pb.CoreErrorReason
	}{
		{"stale", ledger.ErrStaleVersion, codes.Aborted, pb.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT},
		{"key conflict", ledger.ErrIdempotencyConflict, codes.AlreadyExists, pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT},
		{"EOD conflict", domain.ErrBalanceConflict, codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_BALANCE_OBSERVATION_CONFLICT},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := From(test.err)
			if status.Code(err) != test.code {
				t.Fatal(err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatal(details)
			}
			detail, ok := details[0].(*pb.CoreErrorDetail)
			if !ok || detail.Reason != test.reason {
				t.Fatal(details)
			}
		})
	}
}
