package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStorageErrorsPreserveCancellationWithoutSecrets(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{name: "success"},
		{name: "cancelled", err: fmt.Errorf("TOP_SECRET: %w", context.Canceled), want: context.Canceled},
		{name: "deadline", err: fmt.Errorf("TOP_SECRET: %w", context.DeadlineExceeded), want: context.DeadlineExceeded},
		{name: "connection failure", err: errors.New("postgres://TOP_SECRET"), want: ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storageError(tc.err)
			if !errors.Is(got, tc.want) {
				t.Fatalf("wrong storage error: %v", got)
			}
			if got != nil && strings.Contains(got.Error(), "TOP_SECRET") {
				t.Fatal("storage error exposed private driver details")
			}
		})
	}
}
