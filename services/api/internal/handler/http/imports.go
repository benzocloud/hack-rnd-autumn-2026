package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"

	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/core"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

var errImportInput = errors.New("invalid import input")
var errImportSize = errors.New("import payload too large")

func (h *handler) importRoute(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/companies/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/api/v1/companies/") || len(parts) < 2 || parts[1] != "imports" {
		return false
	}
	method := http.MethodGet
	var action http.HandlerFunc
	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		method, action = http.MethodPost, h.uploadImport
	case len(parts) == 2:
		action = h.listImports
	case len(parts) == 3:
		action = h.getImport
	case len(parts) == 4 && parts[3] == "issues":
		action = h.listImportIssues
	case len(parts) == 4 && parts[3] == "commit":
		method, action = http.MethodPost, h.commitImport
	default:
		return false
	}
	if r.Method != method {
		allow := method
		if len(parts) == 2 {
			allow = "GET, POST"
		}
		w.Header().Set("Allow", allow)
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !resourceID(parts[0]) || len(parts) >= 3 && !resourceID(parts[2]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if method == http.MethodPost && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	if h.core.Imports == nil || h.core.Ledger == nil || method == http.MethodPost && len(parts) == 2 && h.core.ImportSpool == nil {
		dependencyError(w, r)
		return true
	}
	timeout := 10 * time.Second
	if method == http.MethodPost {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("company_id", parts[0])
	if len(parts) >= 3 {
		r.SetPathValue("import_id", parts[2])
	}
	action(w, r)
	return true
}

func (h *handler) uploadImport(w http.ResponseWriter, r *http.Request) {
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validIdempotencyKey(keys[0]) || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if !h.authorizeImportWrite(w, r) {
		return
	}
	file, available, err := h.core.ImportSpool.acquire()
	if err != nil {
		dependencyError(w, r)
		return
	}
	if !available {
		w.Header().Set("Retry-After", "1")
		writeError(w, r, 429, "RATE_LIMITED", nil)
		return
	}
	defer func() {
		if err := h.core.ImportSpool.release(file); err != nil {
			h.logger.ErrorContext(r.Context(), "import spool cleanup failed")
		}
	}()
	metadata, err := spoolImport(w, r, file, account.Id)
	if err != nil {
		writeImportInputError(w, r, err)
		return
	}
	metadata.Context, metadata.Scope, metadata.IdempotencyKey = writeContext(r), scopeFor(account), keys[0]
	response, err := coreclient.UploadFile(r.Context(), h.core.Imports, metadata, file)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	accepted := response.GetAccepted()
	if accepted == nil || !resourceID(accepted.Id) || accepted.Status != corev1.ImportStatus_IMPORT_STATUS_QUEUED || accepted.PollAfterMs < 1000 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 202, object{"id": accepted.Id, "status": "queued", "status_url": "/api/v1/companies/" + account.CompanyId + "/imports/" + accepted.Id, "poll_after_ms": accepted.PollAfterMs})
}

func spoolImport(w http.ResponseWriter, r *http.Request, file *os.File, account string) (*corev1.UploadImportMetadata, error) {
	r.Body = http.MaxBytesReader(w, r.Body, importEnvelopeLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, errImportInput
	}
	fields := make(map[string]string)
	seen := make(map[string]bool)
	digest := sha256.New()
	var size int64
	for {
		part, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, importReadError(err)
		}
		name := part.FormName()
		if part.Header.Get("Content-Transfer-Encoding") != "" || seen[name] || !contains(name, "file", "adapter", "source_company_id", "account_id", "currency") || name != "file" && part.FileName() != "" {
			_ = part.Close()
			return nil, errImportInput
		}
		seen[name] = true
		if name == "file" {
			size, err = io.Copy(io.MultiWriter(file, digest), io.LimitReader(&importContextReader{ctx: r.Context(), reader: part}, importFileLimit+1))
			if size > importFileLimit {
				_ = part.Close()
				return nil, errImportSize
			}
		} else {
			var data []byte
			data, err = io.ReadAll(io.LimitReader(part, 513))
			if len(data) > 512 {
				_ = part.Close()
				return nil, errImportInput
			}
			fields[name] = string(data)
		}
		closeErr := part.Close()
		if err != nil {
			return nil, importReadError(err)
		}
		if closeErr != nil {
			return nil, importReadError(closeErr)
		}
	}
	if len(seen) != 5 || size == 0 || fields["adapter"] != "dataset_v1" || fields["currency"] != "RUB" || fields["account_id"] != account || !nonempty(fields["source_company_id"], 128) {
		return nil, errImportInput
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &corev1.UploadImportMetadata{Adapter: fields["adapter"], Currency: fields["currency"], SourceCompanyId: fields["source_company_id"], ByteSize: uint64(size), FileSha256: hex.EncodeToString(digest.Sum(nil))}, nil
}

type importContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *importContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func writeImportInputError(w http.ResponseWriter, r *http.Request, err error) {
	var bodyLimit *http.MaxBytesError
	if errors.Is(err, errImportSize) || errors.As(err, &bodyLimit) {
		writeError(w, r, 413, "PAYLOAD_TOO_LARGE", nil)
		return
	}
	if errors.Is(err, errImportInput) || errors.Is(err, io.ErrUnexpectedEOF) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	dependencyError(w, r)
}

func (h *handler) listImports(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Imports.ListImports(r.Context(), &corev1.ListImportsRequest{Context: browser(r), Scope: scopeFor(account), Page: page})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := make([]object, 0, len(response.Items))
	seen := make(map[string]bool)
	for _, item := range response.Items {
		value, err := importDTO(item, account.Id)
		if err != nil || seen[item.Id] {
			dependencyError(w, r)
			return
		}
		seen[item.Id] = true
		items = append(items, value)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}

func (h *handler) getImport(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Imports.GetImport(r.Context(), &corev1.GetImportRequest{Context: browser(r), Scope: scopeFor(account), ImportId: r.PathValue("import_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := importDTO(response.GetImport(), account.Id)
	if err != nil || response.GetImport().GetId() != r.PathValue("import_id") {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}

func (h *handler) listImportIssues(w http.ResponseWriter, r *http.Request) {
	page, _, err := readPage(r, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Imports.ListImportIssues(r.Context(), &corev1.ListImportIssuesRequest{Context: browser(r), Scope: scopeFor(account), ImportId: r.PathValue("import_id"), Page: page})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items, err := importIssuesDTO(response.Items)
	if err != nil {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}

func importReadError(err error) error {
	var limit *http.MaxBytesError
	var path *os.PathError
	if errors.As(err, &limit) || errors.As(err, &path) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errImportInput
}

func (h *handler) authorizeImportWrite(w http.ResponseWriter, r *http.Request) bool {
	authorizer, ok := h.auth.(interface {
		AuthorizeCompany(context.Context, *authv1.AuthorizeCompanyRequest, ...grpc.CallOption) (*authv1.AuthorizeCompanyResponse, error)
	})
	if !ok {
		dependencyError(w, r)
		return false
	}
	response, err := authorizer.AuthorizeCompany(r.Context(), &authv1.AuthorizeCompanyRequest{
		Context:   &authv1.AuthorizeCompanyRequest_Write{Write: writeContext(r)},
		CompanyId: r.PathValue("company_id"), Permission: authv1.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE,
	})
	if err != nil {
		rpcError(w, r, err)
		return false
	}
	if response == nil || !nonempty(response.ActorId, 128) || roleName(response.Role) == "" {
		dependencyError(w, r)
		return false
	}
	return true
}

func (h *handler) commitImport(w http.ResponseWriter, r *http.Request) {
	keys := r.Header.Values("Idempotency-Key")
	fields, err := ledgerBody(w, r, "version", "expected_data_version", "acknowledge_warnings")
	if err != nil || r.URL.RawQuery != "" || len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	version, validVersion := ledgerVersion(fields["version"])
	dataVersion, validDataVersion := ledgerVersion(fields["expected_data_version"])
	ackRaw := bytes.TrimSpace(fields["acknowledge_warnings"])
	if !validVersion || !validDataVersion || !bytes.Equal(ackRaw, []byte("true")) && !bytes.Equal(ackRaw, []byte("false")) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Imports.CommitImport(r.Context(), &corev1.CommitImportRequest{Context: writeContext(r), Scope: scopeFor(account), ImportId: r.PathValue("import_id"), IdempotencyKey: keys[0], ExpectedVersion: version, ExpectedDataVersion: dataVersion, AcknowledgeWarnings: bytes.Equal(ackRaw, []byte("true"))})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, err := importDTO(response.GetImport(), account.Id)
	if err != nil || response.GetImport().GetId() != r.PathValue("import_id") || response.GetImport().GetStatus() != corev1.ImportStatus_IMPORT_STATUS_APPLIED {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
