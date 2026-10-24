package testauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

const (
	Email          = "demo@example.test"
	Password       = "example-password-only"
	Code           = "001234"
	ResetToken     = "example-one-time-token"
	ChallengeID    = "550e8400-e29b-41d4-a716-446655440000"
	ConsentText    = "# Synthetic API fixture consent\n\nVersion: api-fixture-v1\nOnly synthetic test data; no email delivery or persistent account creation.\n"
	ConsentVersion = "api-fixture-v1"
)

type browserState struct {
	csrf    string
	expires time.Time
	session bool
}

// Server is a synthetic transport fixture, not the production Auth service.
// It deliberately omits persistence, hashing, delivery and production policies.
type Server struct {
	authv1.UnimplementedAuthServiceServer
	mu        sync.Mutex
	contexts  map[string]browserState
	failures  map[string]error
	delays    map[string]time.Duration
	requests  map[string][]proto.Message
	challenge string
	verified  bool
	password  string
	resetUsed bool
}

func New() *Server {
	return &Server{
		contexts: make(map[string]browserState), failures: make(map[string]error),
		delays: make(map[string]time.Duration), requests: make(map[string][]proto.Message),
		challenge: ChallengeID, password: Password,
	}
}

// SetFailure injects a synthetic RPC failure; nil clears it.
func (s *Server) SetFailure(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[method] = err
}

func (s *Server) SetDelay(method string, delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delays[method] = delay
}

// Requests returns independent snapshots of messages actually received by RPC.
func (s *Server) Requests(method string) []proto.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]proto.Message, len(s.requests[method]))
	for i, request := range s.requests[method] {
		result[i] = proto.Clone(request)
	}
	return result
}

func (s *Server) before(ctx context.Context, method string, request proto.Message) error {
	s.mu.Lock()
	s.requests[method] = append(s.requests[method], proto.Clone(request))
	delay, failure := s.delays[method], s.failures[method]
	s.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	return failure
}

func ConsentDocument() *authv1.ConsentDocument {
	return &authv1.ConsentDocument{
		Text: ConsentText, DocumentVersion: ConsentVersion,
		DocumentSha256: fmt.Sprintf("%x", sha256.Sum256([]byte(ConsentText))),
	}
}

func (s *Server) GetPersonalDataConsent(ctx context.Context, request *authv1.GetPersonalDataConsentRequest) (*authv1.GetPersonalDataConsentResponse, error) {
	if err := s.before(ctx, "GetPersonalDataConsent", request); err != nil {
		return nil, err
	}
	return &authv1.GetPersonalDataConsentResponse{Document: ConsentDocument()}, nil
}

func (s *Server) GetSecurityContext(ctx context.Context, request *authv1.GetSecurityContextRequest) (*authv1.GetSecurityContextResponse, error) {
	if err := s.before(ctx, "GetSecurityContext", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cookie := request.GetContext().GetCookieToken()
	state, ok := s.contexts[cookie]
	if !ok || !state.expires.After(time.Now()) {
		delete(s.contexts, cookie)
		cookie, state = s.newContext(false)
	}
	return &authv1.GetSecurityContextResponse{Context: securityContext(cookie, state)}, nil
}

func (s *Server) Register(ctx context.Context, request *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	if err := s.before(ctx, "Register", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	acceptance, document := request.GetPersonalDataConsent(), ConsentDocument()
	if !acceptance.GetAccepted() {
		return nil, fixtureError(codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_REQUIRED)
	}
	if acceptance.GetDocumentVersion() != document.GetDocumentVersion() || acceptance.GetDocumentSha256() != document.GetDocumentSha256() {
		return nil, fixtureError(codes.Aborted, authv1.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_DOCUMENT_CHANGED)
	}
	return &authv1.RegisterResponse{Challenge: s.challengeResponse()}, nil
}

func (s *Server) Login(ctx context.Context, request *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	if err := s.before(ctx, "Login", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(request.GetEmail())) != Email || request.GetPassword() != s.password {
		return nil, fixtureError(codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS)
	}
	delete(s.contexts, request.GetContext().GetBrowser().GetCookieToken())
	cookie, state := s.newContext(true)
	return &authv1.LoginResponse{User: s.profile(), Context: securityContext(cookie, state)}, nil
}

func (s *Server) VerifyEmail(ctx context.Context, request *authv1.VerifyEmailRequest) (*authv1.VerifyEmailResponse, error) {
	if err := s.before(ctx, "VerifyEmail", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	if request.GetChallengeId() != s.challenge || request.GetCode() != Code {
		return nil, fixtureError(codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_CODE_INVALID_OR_EXPIRED)
	}
	s.verified = true
	return &authv1.VerifyEmailResponse{}, nil
}

func (s *Server) ResendVerification(ctx context.Context, request *authv1.ResendVerificationRequest) (*authv1.ResendVerificationResponse, error) {
	if err := s.before(ctx, "ResendVerification", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	s.challenge = randomUUID()
	return &authv1.ResendVerificationResponse{Challenge: s.challengeResponse()}, nil
}

func (s *Server) ForgotPassword(ctx context.Context, request *authv1.ForgotPasswordRequest) (*authv1.ForgotPasswordResponse, error) {
	if err := s.before(ctx, "ForgotPassword", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	s.resetUsed = false
	return &authv1.ForgotPasswordResponse{}, nil
}

func (s *Server) ResetPassword(ctx context.Context, request *authv1.ResetPasswordRequest) (*authv1.ResetPasswordResponse, error) {
	if err := s.before(ctx, "ResetPassword", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	if request.GetToken() != ResetToken || s.resetUsed {
		return nil, fixtureError(codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_TOKEN_INVALID_OR_EXPIRED)
	}
	s.password, s.resetUsed = request.GetNewPassword(), true
	clear(s.contexts)
	return &authv1.ResetPasswordResponse{}, nil
}

func (s *Server) Logout(ctx context.Context, request *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if err := s.before(ctx, "Logout", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorizeWrite(request.GetContext()); err != nil {
		return nil, err
	}
	delete(s.contexts, request.GetContext().GetBrowser().GetCookieToken())
	return &authv1.LogoutResponse{}, nil
}

func (s *Server) GetCurrentAccount(ctx context.Context, request *authv1.GetCurrentAccountRequest) (*authv1.GetCurrentAccountResponse, error) {
	if err := s.before(ctx, "GetCurrentAccount", request); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.contexts[request.GetContext().GetCookieToken()]
	if !ok || !state.session || !state.expires.After(time.Now()) {
		return nil, fixtureError(codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED)
	}
	return &authv1.GetCurrentAccountResponse{User: s.profile()}, nil
}

// Caller holds mu for all state helpers.
func (s *Server) authorizeWrite(write *authv1.WriteContext) error {
	state, ok := s.contexts[write.GetBrowser().GetCookieToken()]
	if !ok || !state.expires.After(time.Now()) || subtle.ConstantTimeCompare([]byte(write.GetCsrfToken()), []byte(state.csrf)) != 1 {
		return fixtureError(codes.PermissionDenied, authv1.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID)
	}
	return nil
}

func (s *Server) newContext(session bool) (string, browserState) {
	ttl := 10 * time.Minute
	if session {
		ttl = 24 * time.Hour
	}
	cookie := randomToken()
	state := browserState{csrf: randomToken(), expires: time.Now().Add(ttl), session: session}
	s.contexts[cookie] = state
	return cookie, state
}

func securityContext(cookie string, state browserState) *authv1.SecurityContext {
	return &authv1.SecurityContext{CookieToken: cookie, CsrfToken: state.csrf, CookieExpiresAt: timestamppb.New(state.expires)}
}

func (s *Server) challengeResponse() *authv1.EmailVerificationChallenge {
	return &authv1.EmailVerificationChallenge{ChallengeId: s.challenge, ExpiresAt: timestamppb.New(time.Now().Add(10 * time.Minute))}
}

func (s *Server) profile() *authv1.UserProfile {
	return &authv1.UserProfile{Id: "550e8400-e29b-41d4-a716-446655440001", Email: Email, DisplayName: "API Demo", EmailVerified: s.verified}
}

func fixtureError(code codes.Code, reason authv1.AuthErrorReason) error {
	st, err := status.New(code, "synthetic Auth rejection").WithDetails(&authv1.AuthErrorDetail{Reason: reason})
	if err != nil {
		return status.Error(codes.Internal, "fixture error detail failure")
	}
	return st.Err()
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("test fixture random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func randomUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("test fixture random source unavailable")
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}
