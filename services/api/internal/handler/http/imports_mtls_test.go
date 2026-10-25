package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/core"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

// This verifies the actual HTTP/protobuf/mTLS stream, not Core persistence.
type importRPCFixture struct {
	corev1.UnimplementedImportsServiceServer
	mu       sync.Mutex
	metadata *corev1.UploadImportMetadata
	accepted int
}

func (f *importRPCFixture) UploadImport(stream grpc.ClientStreamingServer[corev1.UploadImportRequest, corev1.UploadImportResponse]) error {
	first, err := stream.Recv()
	if err != nil || first.GetMetadata() == nil {
		return status.Error(codes.InvalidArgument, "metadata required")
	}
	metadata := first.GetMetadata()
	if metadata.GetContext().GetBrowser().GetCookieToken() != "opaque-session" || metadata.GetContext().GetCsrfToken() != strings.Repeat("c", 24) || metadata.GetScope().GetAccountId() != accountID || metadata.GetScope().GetCompanyId() != companyID {
		return status.Error(codes.PermissionDenied, "context lost")
	}
	digest := sha256.New()
	var size uint64
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if frame.GetMetadata() != nil || len(frame.GetChunk()) == 0 || len(frame.GetChunk()) > 1<<20 {
			return status.Error(codes.InvalidArgument, "bad chunk")
		}
		_, _ = digest.Write(frame.GetChunk())
		size += uint64(len(frame.GetChunk()))
	}
	if size != metadata.ByteSize || hex.EncodeToString(digest.Sum(nil)) != metadata.FileSha256 {
		return status.Error(codes.InvalidArgument, "bytes changed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.metadata != nil && f.metadata.IdempotencyKey == metadata.IdempotencyKey && f.metadata.FileSha256 != metadata.FileSha256 {
		st, _ := status.New(codes.AlreadyExists, "PRIVATE UPSTREAM").WithDetails(&corev1.CoreErrorDetail{Reason: corev1.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT})
		return st.Err()
	}
	f.metadata = proto.Clone(metadata).(*corev1.UploadImportMetadata)
	f.accepted++
	return stream.SendAndClose(&corev1.UploadImportResponse{Accepted: &corev1.ImportAccepted{Id: forecastID, Status: corev1.ImportStatus_IMPORT_STATUS_QUEUED, PollAfterMs: 1000}})
}

func TestImportHTTPSAndCoreMTLS(t *testing.T) {
	pki, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := securetls.Load(pki.CA, pki.CoreCert, pki.CoreKey, "spiffe://saldo.local/api", true)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &importRPCFixture{}
	financial := &financialRPCFixture{f: newFinancialFixture()}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(identity)), grpc.MaxRecvMsgSize(2<<20))
	corev1.RegisterImportsServiceServer(server, fixture)
	corev1.RegisterLedgerServiceServer(server, financial)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := coreclient.Dial(listener.Addr().String(), "localhost", pki.CA, pki.APICert, pki.APIKey, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	spool, err := NewImportSpool(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	auth := &importAuthFixture{financialFixture: newFinancialFixture()}
	var handler http.Handler
	https := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(https.Close)
	handler = NewWithCore(auth, https.URL, slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Ledger: corev1.NewLedgerServiceClient(connection), Imports: corev1.NewImportsServiceClient(connection), ImportSpool: spool})
	data := []byte(strings.Repeat("CSV-DATA;", 70000))
	upload := func(data []byte, csrf string) (int, string) {
		t.Helper()
		body, contentType := importMultipart(t, data, nil, "")
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, https.URL+financialRoot+"/imports", body)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Origin", https.URL)
		request.Header.Set("X-CSRF-Token", csrf)
		request.Header.Set("Idempotency-Key", "upload-key-123")
		request.AddCookie(&http.Cookie{Name: "__Host-saldo_session", Value: "opaque-session"})
		response, err := https.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
		return response.StatusCode, string(result)
	}
	for range 2 {
		code, body := upload(data, strings.Repeat("c", 24))
		if code != 202 {
			t.Fatal(code, body)
		}
		var accepted map[string]any
		if err := json.Unmarshal([]byte(body), &accepted); err != nil {
			t.Fatal(err)
		}
		if accepted["id"] != forecastID || accepted["status"] != "queued" {
			t.Fatal(accepted)
		}
	}
	if code, body := upload([]byte("changed file"), strings.Repeat("c", 24)); code != 409 || strings.Contains(body, "PRIVATE") {
		t.Fatal(code, body)
	}
	fixture.mu.Lock()
	count := fixture.accepted
	fixture.mu.Unlock()
	if code, _ := upload(data, ""); code != 403 {
		t.Fatal("CSRF missing", code)
	}
	auth.authorizeErr = status.Error(codes.PermissionDenied, "PRIVATE")
	if code, _ := upload(data, strings.Repeat("c", 24)); code != 503 {
		t.Fatal("write authorization not fail-closed", code)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.accepted != count {
		t.Fatal("unauthorized body reached Core")
	}
	entries, err := os.ReadDir(spool.root)
	if err != nil || len(entries) != 0 || spool.slots != 0 {
		t.Fatal("spool bytes/reservation leaked", err)
	}
}
