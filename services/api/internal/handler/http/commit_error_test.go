package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestStaleImportPreviewMapsAbortedToConflict(t *testing.T) {
	for _, test := range []struct {
		code codes.Code
		want int
	}{{codes.Aborted, 409}, {codes.FailedPrecondition, 503}} {
		st, err := status.New(test.code, "private upstream diagnostic").WithDetails(&corev1.CoreErrorDetail{Reason: corev1.CoreErrorReason_CORE_ERROR_REASON_IMPORT_PREVIEW_STALE})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		coreRPCError(response, httptest.NewRequest("POST", "/api/v1/companies/company/imports/import/commit", nil), st.Err())
		if response.Code != test.want || strings.Contains(response.Body.String(), "private upstream") {
			t.Fatal(response.Code, response.Body.String())
		}
		if test.want == 409 && !strings.Contains(response.Body.String(), `"code":"IMPORT_PREVIEW_STALE"`) {
			t.Fatal("stale error was hidden", response.Body.String())
		}
	}
}
