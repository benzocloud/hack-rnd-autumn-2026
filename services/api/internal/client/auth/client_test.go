package auth

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

func TestDeadlines(t *testing.T) {
	for _, method := range []string{authv1.AuthService_Login_FullMethodName, authv1.AuthService_Register_FullMethodName, authv1.AuthService_ResetPassword_FullMethodName, authv1.AuthService_GetCurrentAccount_FullMethodName} {
		want := time.Second
		if method != authv1.AuthService_GetCurrentAccount_FullMethodName {
			want = 2 * time.Second
		}
		err := deadlines(time.Second, 2*time.Second)(t.Context(), method, nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			d, ok := ctx.Deadline()
			if !ok || time.Until(d) > want || time.Until(d) < want-100*time.Millisecond {
				t.Fatal("wrong deadline")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_ = deadlines(time.Second, time.Second)(ctx, "method", nil, nil, nil, func(call context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		d, _ := call.Deadline()
		parent, _ := ctx.Deadline()
		if !d.Equal(parent) {
			t.Fatal("parent deadline extended")
		}
		return nil
	})
	if _, _, err := Dial("127.0.0.1:1", "missing", "missing", "missing", time.Second, time.Second); err == nil {
		t.Fatal("missing mTLS identity accepted")
	}
}
