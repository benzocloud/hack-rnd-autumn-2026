package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func importMultipart(t *testing.T, data []byte, changes map[string]string, duplicate string) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	fields := map[string]string{"adapter": "dataset_v1", "source_company_id": "C001", "account_id": accountID, "currency": "RUB"}
	for key, value := range changes {
		fields[key] = value
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if duplicate != "" {
		if err := writer.WriteField(duplicate, "again"); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("file", "../../private.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

func TestImportSpoolMultipartValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		data      []byte
		changes   map[string]string
		duplicate string
		want      int
	}{
		{name: "valid", data: []byte("private CSV bytes"), want: 0},
		{name: "empty", want: 422},
		{name: "wrong account", data: []byte("x"), changes: map[string]string{"account_id": companyID}, want: 422},
		{name: "unsupported adapter", data: []byte("x"), changes: map[string]string{"adapter": "arbitrary"}, want: 422},
		{name: "unknown field", data: []byte("x"), changes: map[string]string{"certified": "true"}, want: 422},
		{name: "duplicate field", data: []byte("x"), duplicate: "source_company_id", want: 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			spool, err := NewImportSpool(filepath.Join(t.TempDir(), "private"))
			if err != nil {
				t.Fatal(err)
			}
			file, _, err := spool.acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = spool.release(file) }()
			body, contentType := importMultipart(t, test.data, test.changes, test.duplicate)
			r := httptest.NewRequest(http.MethodPost, financialRoot+"/imports", body)
			r.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()
			metadata, err := spoolImport(w, r, file, accountID)
			if test.want != 0 {
				if err == nil {
					t.Fatal("accepted invalid multipart")
				}
				writeImportInputError(w, r, err)
				if w.Code != test.want {
					t.Fatal(w.Code, w.Body.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(test.data)
			actual, err := io.ReadAll(file)
			if err != nil || !bytes.Equal(actual, test.data) || metadata.ByteSize != uint64(len(test.data)) || metadata.FileSha256 != hex.EncodeToString(digest[:]) {
				t.Fatal("changed uploaded bytes/hash", err)
			}
			info, err := file.Stat()
			if err != nil || info.Mode().Perm() != 0o600 || filepath.Dir(file.Name()) != spool.root {
				t.Fatal("unsafe spool", err)
			}
		})
	}
}

func TestImportSpoolQuotaAndStartupCleanup(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, importSpoolPrefix+"old")
	if err := os.WriteFile(orphan, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	spool, err := NewImportSpool(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan was not removed")
	}
	one, _, _ := spool.acquire()
	two, _, _ := spool.acquire()
	if file, ok, err := spool.acquire(); file != nil || ok || err != nil {
		t.Fatal("quota did not reject third upload")
	}
	if err := spool.release(one); err != nil {
		t.Fatal(err)
	}
	three, ok, err := spool.acquire()
	if err != nil || !ok {
		t.Fatal("deleted bytes did not release reservation", err)
	}
	if err := spool.release(two); err != nil {
		t.Fatal(err)
	}
	if err := spool.release(three); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("spool leaked files")
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewImportSpool(root); err == nil {
		t.Fatal("public directory accepted")
	}
}

func TestImportSpoolReleaseFreesSlotWhenUnlinkFails(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	spool, err := NewImportSpool(root)
	if err != nil {
		t.Fatal(err)
	}
	file, ok, err := spool.acquire()
	if err != nil || !ok || file == nil {
		t.Fatal(err)
	}
	name := file.Name()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if err := spool.release(file); err == nil {
		t.Fatal("unlink succeeded in a directory without write permission")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	next, ok, err := spool.acquire()
	if err != nil || !ok || next == nil {
		t.Fatal("slot stayed reserved after failed unlink", err)
	}
	if err := spool.release(next); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

type importAuthFixture struct {
	*financialFixture
	authorizeErr   error
	authorizations int
}

func (f *importAuthFixture) AuthorizeCompany(_ context.Context, r *authv1.AuthorizeCompanyRequest, _ ...grpc.CallOption) (*authv1.AuthorizeCompanyResponse, error) {
	f.authorizations++
	if r.CompanyId != companyID || r.Permission != authv1.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE || r.GetWrite().GetCsrfToken() != strings.Repeat("c", 24) {
		return nil, status.Error(codes.PermissionDenied, "private")
	}
	return &authv1.AuthorizeCompanyResponse{ActorId: companyID, Role: authv1.CompanyRole_COMPANY_ROLE_OWNER}, f.authorizeErr
}

type importsReadFixture struct {
	corev1.ImportsServiceClient
	item    *corev1.Import
	listErr error
}

func (f *importsReadFixture) GetImport(_ context.Context, r *corev1.GetImportRequest, _ ...grpc.CallOption) (*corev1.GetImportResponse, error) {
	if r.Scope.AccountId != accountID || r.Context.CookieToken != "opaque-session" {
		return nil, status.Error(codes.InvalidArgument, "scope lost")
	}
	return &corev1.GetImportResponse{Import: f.item}, f.listErr
}
func (f *importsReadFixture) ListImports(context.Context, *corev1.ListImportsRequest, ...grpc.CallOption) (*corev1.ListImportsResponse, error) {
	return &corev1.ListImportsResponse{Items: []*corev1.Import{f.item}, Pagination: &corev1.PageResponse{}}, f.listErr
}
func (f *importsReadFixture) ListImportIssues(context.Context, *corev1.ListImportIssuesRequest, ...grpc.CallOption) (*corev1.ListImportIssuesResponse, error) {
	return &corev1.ListImportIssuesResponse{Items: []*corev1.ImportIssue{{Code: "INVALID_ROW", Message: "Строка не соответствует формату", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_BLOCKING}}, Pagination: &corev1.PageResponse{}}, f.listErr
}

func queuedImport() *corev1.Import {
	return &corev1.Import{Id: forecastID, AccountId: accountID, SourceKind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE, Adapter: "dataset_v1", SourceCompanyId: "C001", Status: corev1.ImportStatus_IMPORT_STATUS_QUEUED, Version: 1, CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now(), RecordKinds: []corev1.ImportRecordKind{corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS, corev1.ImportRecordKind_IMPORT_RECORD_KIND_BALANCE_OBSERVATIONS}}
}

func TestImportReadRoutesAndFailClosedProjection(t *testing.T) {
	f := newFinancialFixture()
	imports := &importsReadFixture{item: queuedImport()}
	h := NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Ledger: f, Imports: imports})
	for _, path := range []string{financialRoot + "/imports", financialRoot + "/imports/" + forecastID, financialRoot + "/imports/" + forecastID + "/issues"} {
		w := financialRequest(h, "GET", path, "")
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{financialRoot + "/imports?limit=0", financialRoot + "/imports?cursor=a&cursor=b", financialRoot + "/imports/" + forecastID + "?unexpected=x"} {
		if w := financialRequest(h, "GET", path, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	imports.item.AccountId = companyID
	if w := financialRequest(h, "GET", financialRoot+"/imports/"+forecastID, ""); w.Code != 503 {
		t.Fatal("foreign account projected", w.Code)
	}
	imports.item = nil
	if w := financialRequest(h, "GET", financialRoot+"/imports", ""); w.Code != 503 {
		t.Fatal("nil resource panicked or projected", w.Code)
	}
	if w := financialRequest(h, "POST", financialRoot+"/imports/"+forecastID+"/commit", `{}`); w.Code != 422 {
		t.Fatal("empty commit body not rejected", w.Code)
	}
}

type importZeroReader struct{}

func (importZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestImportOversizeStreamingBody(t *testing.T) {
	spool, err := NewImportSpool(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := spool.acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = spool.release(file) }()
	body := io.MultiReader(strings.NewReader("--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x.csv\"\r\n\r\n"), io.LimitReader(importZeroReader{}, importFileLimit+1), strings.NewReader("\r\n--boundary--\r\n"))
	r := httptest.NewRequest(http.MethodPost, financialRoot+"/imports", body)
	r.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	w := httptest.NewRecorder()
	_, err = spoolImport(w, r, file, accountID)
	if err == nil {
		t.Fatal("accepted oversize")
	}
	writeImportInputError(w, r, err)
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func (f *importsReadFixture) CommitImport(context.Context, *corev1.CommitImportRequest, ...grpc.CallOption) (*corev1.CommitImportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "fixture commit unavailable")
}
