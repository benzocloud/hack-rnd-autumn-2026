package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/status"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestPublicErrorsHaveKnownMessagesWithoutUpstreamDiagnostics(t *testing.T) {
	for value, name := range authv1.AuthErrorReason_name {
		if value == 0 {
			continue
		}
		reason := strings.TrimPrefix(name, "AUTH_ERROR_REASON_")
		code, _ := reasonStatus(reason)
		st, err := status.New(code, "private: email=secret@example.test password=SECRET").WithDetails(&authv1.AuthErrorDetail{Reason: authv1.AuthErrorReason(value)})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		rpcError(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil), st.Err())
		var body errorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Message != errorMessage(reason) || strings.Contains(response.Body.String(), "SECRET") || strings.Contains(response.Body.String(), "secret@example.test") {
			t.Fatal(reason, response.Body.String())
		}
		if reason != "INTERNAL_ERROR" && body.Error.Message == errorMessage("UNKNOWN") {
			t.Fatal("missing public Auth message", reason)
		}
	}
	for _, name := range corev1.CoreErrorReason_name {
		reason := strings.TrimPrefix(name, "CORE_ERROR_REASON_")
		if _, code := coreReasonStatus(reason); code == 0 || reason == "INTERNAL_ERROR" {
			continue
		}
		if errorMessage(reason) == errorMessage("UNKNOWN") {
			t.Fatal("missing public Core message", reason)
		}
	}
}

func TestLoginMessageDoesNotDiscloseAccountExistence(t *testing.T) {
	code, _ := reasonStatus("INVALID_CREDENTIALS")
	for _, diagnostic := range []string{"email not registered", "password mismatch"} {
		st, err := status.New(code, diagnostic).WithDetails(&authv1.AuthErrorDetail{Reason: authv1.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		rpcError(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil), st.Err())
		var body errorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != 401 || body.Error.Message != "Неверная почта или пароль. Проверьте данные и попробуйте ещё раз." || strings.Contains(response.Body.String(), diagnostic) {
			t.Fatal(response.Code, response.Body.String())
		}
	}
}
