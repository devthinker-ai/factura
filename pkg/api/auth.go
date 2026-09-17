package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/apikeys"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/session"
)

const minPasswordLen = 10

func (s *Server) mountAuth(r chi.Router) {
	// Public (precede a session).
	r.Post("/login", s.handleLogin)
	r.Post("/login/mfa", s.handleLoginMFA)
	r.Get("/auth-status", s.handleAuthStatus)
	// First-user bootstrap is public when table empty; otherwise admin-only
	// (checked inside — registered outside the auth group intentionally).
	r.Post("/users", s.handleUsersCreate)
}

func (s *Server) mountAuthAuthed(r chi.Router) {
	r.Get("/me", s.handleMe)
	r.Get("/users", s.handleUsersList)
	r.Patch("/users/{id}", s.handleUsersPatch)
	r.Delete("/users/{id}", s.handleUsersDelete)
	r.Post("/2fa/setup", s.handle2FASetup)
	r.Post("/2fa/confirm", s.handle2FAConfirm)
	r.Delete("/2fa", s.handle2FADelete)
}

// authMode returns open|token|users for /health and /auth-status.
func (s *Server) authMode() string {
	if s.usersExist() {
		return "users"
	}
	if s.Token != "" {
		return "token"
	}
	return "open"
}

func (s *Server) usersExist() bool {
	s.usersMu.Lock()
	defer s.usersMu.Unlock()
	// Cache for 30 s; bumped immediately on AddUser/RemoveUser.
	if s.usersCached && time.Since(s.usersChecked) < 30*time.Second {
		return s.usersExists
	}
	n, err := s.Store.CountUsers()
	if err != nil {
		return s.usersExists
	}
	s.usersExists = n > 0
	s.usersChecked = time.Now()
	s.usersCached = true
	return s.usersExists
}

func (s *Server) bumpUsersExist(exists bool) {
	s.usersMu.Lock()
	s.usersExists = exists
	s.usersChecked = time.Now()
	s.usersCached = true
	s.usersMu.Unlock()
}

// BumpUsersCache forces the users-exist cache (tests).
func (s *Server) BumpUsersCache(exists bool) {
	s.bumpUsersExist(exists)
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"mode": s.authMode()})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if ok, retry := s.loginLim.Allowed(email); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
		return
	}
	u, err := s.Store.UserByEmail(email)
	if err != nil || !session.VerifyHash(u.PasswordHash, body.Password) {
		s.loginLim.RecordFail(email)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	s.loginLim.Clear(email)
	if u.TOTPEnabled {
		tok, err := s.Sessions.IssueMFA(u.ID)
		if err != nil {
			s.log.Printf("issue mfa: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    tok,
		})
		return
	}
	tok, err := s.Sessions.Issue(session.User{ID: u.ID, Email: u.Email, Role: u.Role, Name: u.Name})
	if err != nil {
		s.log.Printf("issue session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  userDTO(u),
	})
}

func (s *Server) handleLoginMFA(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	claims, err := s.Sessions.ParseMFA(body.MFAToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "mfa token expired or already used"})
		return
	}
	exp := time.Time{}
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	if s.mfaGate.IsUsed(claims.ID) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "mfa token expired or already used"})
		return
	}
	allowed, _ := s.mfaGate.CheckAndCountAttempt(claims.ID, exp)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(session.MFARetryAfterSec))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
		return
	}

	u, err := s.Store.GetUser(claims.Subject)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	code := strings.TrimSpace(body.Code)
	ok := false
	if session.LooksLikeRecoveryCode(code) {
		ok, err = s.Store.ConsumeRecoveryCode(u.ID, session.HashRecoveryCode(code))
		if err != nil {
			s.log.Printf("consume recovery: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
	} else {
		ok, _ = session.Verify(u.TOTPSecret, code, s.now())
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	if !s.mfaGate.MarkUsed(claims.ID, exp) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "mfa token expired or already used"})
		return
	}
	tok, err := s.Sessions.Issue(session.User{ID: u.ID, Email: u.Email, Role: u.Role, Name: u.Name})
	if err != nil {
		s.log.Printf("issue session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  userDTO(u),
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	au, ok := session.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not a user session"})
		return
	}
	u, err := s.Store.GetUser(au.ID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(u)})
}

func (s *Server) handleUsersList(w http.ResponseWriter, _ *http.Request) {
	users, err := s.Store.ListUsers()
	if err != nil {
		s.log.Printf("list users: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, userDTO(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid body"})
		return
	}
	n, err := s.Store.CountUsers()
	if err != nil {
		s.log.Printf("count users: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if n == 0 {
		// Public first-user bootstrap — must be admin.
		if body.Role == "" {
			body.Role = archive.RoleAdmin
		}
		if body.Role != archive.RoleAdmin {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "first user must be admin",
			})
			return
		}
	} else {
		// Needs admin session (or operator / machine key — role matrix already ran if authed).
		// This route is public; enforce auth here.
		if !s.requireAdminOrOperator(w, r) {
			return
		}
	}
	if len(body.Password) < minPasswordLen {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "password too short",
		})
		return
	}
	if body.Role == "" {
		body.Role = archive.RoleViewer
	}
	if !archive.ValidRole(body.Role) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid role"})
		return
	}
	hash, err := session.FormatHash(body.Password)
	if err != nil {
		s.log.Printf("hash: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	u, err := s.Store.AddUser(body.Email, body.Name, body.Role, hash)
	if errors.Is(err, archive.ErrDuplicateEmail) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already exists"})
		return
	}
	if err != nil {
		s.log.Printf("add user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.bumpUsersExist(true)
	writeJSON(w, http.StatusCreated, userDTO(u))
}

func (s *Server) handleUsersPatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	au, _ := session.UserFromContext(r.Context())
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid body"})
		return
	}
	if body.Role != nil && au.ID == id {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "cannot modify your own role or delete yourself",
		})
		return
	}
	if body.Role != nil {
		if !archive.ValidRole(*body.Role) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid role"})
			return
		}
		if err := s.Store.SetRole(id, *body.Role); err != nil {
			if errors.Is(err, archive.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			s.log.Printf("set role: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
	}
	if body.Password != nil {
		if len(*body.Password) < minPasswordLen {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "password too short"})
			return
		}
		hash, err := session.FormatHash(*body.Password)
		if err != nil {
			s.log.Printf("hash: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if err := s.Store.SetPassword(id, hash); err != nil {
			if errors.Is(err, archive.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			s.log.Printf("set password: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
	}
	u, err := s.Store.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, userDTO(u))
}

func (s *Server) handleUsersDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	au, _ := session.UserFromContext(r.Context())
	if au.ID == id {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "cannot modify your own role or delete yourself",
		})
		return
	}
	if err := s.Store.RemoveUser(id); err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		s.log.Printf("remove user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	n, _ := s.Store.CountUsers()
	s.bumpUsersExist(n > 0)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	au, ok := session.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not a user session"})
		return
	}
	secret, err := session.GenerateSecret()
	if err != nil {
		s.log.Printf("totp secret: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	uri := session.ProvisioningURI(secret, au.Email)
	qr, err := session.QRDataURL(uri)
	if err != nil {
		s.log.Printf("qr: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	// Persist pending secret (enabled=false) so confirm can verify.
	if err := s.Store.UpsertTOTP(au.ID, secret, false, nil, ""); err != nil {
		s.log.Printf("upsert totp: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":           secret,
		"provisioning_uri": uri,
		"qr":               qr,
	})
}

func (s *Server) handle2FAConfirm(w http.ResponseWriter, r *http.Request) {
	au, ok := session.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not a user session"})
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	u, err := s.Store.GetUser(au.ID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	// Already enrolled: do not re-issue recovery codes (idempotent confirm).
	if u.TOTPEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": []string{}})
		return
	}
	if u.TOTPSecret == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	okCode, _ := session.Verify(u.TOTPSecret, body.Code, s.now())
	if !okCode {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}
	plain, err := session.GenerateRecoveryCodes()
	if err != nil {
		s.log.Printf("recovery codes: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	hashes := make([]string, len(plain))
	for i, c := range plain {
		hashes[i] = session.HashRecoveryCode(c)
	}
	confirmed := s.now().UTC().Format(time.RFC3339)
	if err := s.Store.UpsertTOTP(u.ID, u.TOTPSecret, true, hashes, confirmed); err != nil {
		s.log.Printf("confirm totp: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": plain})
}

func (s *Server) handle2FADelete(w http.ResponseWriter, r *http.Request) {
	au, ok := session.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not a user session"})
		return
	}
	_ = s.Store.ClearTOTP(au.ID) // idempotent
	w.WriteHeader(http.StatusNoContent)
}

func userDTO(u archive.User) map[string]any {
	m := map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"name":         u.Name,
		"role":         u.Role,
		"totp_enabled": u.TOTPEnabled,
		"created_at":   u.CreatedAt,
	}
	if u.TOTPConfirmedAt != "" {
		m["totp_confirmed_at"] = u.TOTPConfirmedAt
	}
	return m
}

// requireAdminOrOperator is used by the public POST /users when users already exist.
// Machine keys and the operator token bypass the role matrix (same as Step 1/3a).
func (s *Server) requireAdminOrOperator(w http.ResponseWriter, r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	token := ""
	if strings.HasPrefix(h, prefix) {
		token = strings.TrimPrefix(h, prefix)
	}
	if apikeys.IsAPIKeyToken(token) && s.Keys != nil {
		_, result, retryAfter := s.Keys.Check(token, false)
		switch result {
		case apikeys.CheckUnauthorized:
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return false
		case apikeys.CheckRateLimited:
			apikeys.WriteRateLimited(w, retryAfter)
			return false
		case apikeys.CheckQuotaExceeded:
			apikeys.WriteQuotaExceeded(w)
			return false
		}
		return true
	}
	if s.Token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) == 1 {
		return true
	}
	if s.Sessions == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	claims, err := s.Sessions.Parse(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	if claims.Role != archive.RoleAdmin {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "forbidden"})
		return false
	}
	u, err := s.Store.GetUser(claims.Subject)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	ctx := session.WithUser(r.Context(), session.AuthUser{
		ID: u.ID, Email: u.Email, Role: u.Role, Name: u.Name,
	})
	*r = *r.WithContext(ctx)
	return true
}

// authRoleFor returns the minimum role required for method+path, or "" if any
// authenticated session may proceed (public-within-auth like 2FA self-service
// still needs a session — callers check that separately).
//
// Roles: viewer < editor < admin. Returns the minimum role name.
// Machine keys / operator tokens bypass this matrix entirely.
func authRoleFor(method, path string) string {
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}

	// 2FA self-service — any role.
	if strings.HasPrefix(path, "/2fa") {
		return archive.RoleViewer
	}
	if path == "/me" {
		return archive.RoleViewer
	}

	// User management — admin only.
	if path == "/users" || strings.HasPrefix(path, "/users/") {
		return archive.RoleAdmin
	}

	// Phase 6 license writes + api-keys writes — admin.
	if path == "/license" && (method == http.MethodPost || method == http.MethodDelete) {
		return archive.RoleAdmin
	}
	if path == "/api-keys" && method == http.MethodPost {
		return archive.RoleAdmin
	}
	if strings.HasPrefix(path, "/api-keys/") && (method == http.MethodPatch || method == http.MethodDelete) {
		return archive.RoleAdmin
	}

	// Companies — GET /companies/{id} viewer; writes editor+.
	if strings.HasPrefix(path, "/companies/") && method == http.MethodGet {
		return archive.RoleViewer
	}
	if path == "/companies" && method == http.MethodPost {
		return archive.RoleEditor
	}
	if strings.HasPrefix(path, "/companies/") && method == http.MethodPatch {
		return archive.RoleEditor
	}

	// Phase 6 GETs — viewer.
	if method == http.MethodGet && (path == "/license" || path == "/companies" ||
		path == "/api-keys" || path == "/usage" || path == "/audit") {
		return archive.RoleViewer
	}

	// Peppol.
	if strings.HasPrefix(path, "/peppol/") {
		peppolPath := strings.TrimPrefix(path, "/peppol")
		switch {
		case method == http.MethodGet:
			return archive.RoleViewer
		case peppolPath == "/inbound" && method == http.MethodPost:
			return archive.RoleEditor
		case peppolPath == "/send" && method == http.MethodPost:
			return archive.RoleEditor
		case strings.HasPrefix(peppolPath, "/send/") && strings.HasSuffix(peppolPath, "/retry") && method == http.MethodPost:
			return archive.RoleEditor
		case peppolPath == "/participants" && method == http.MethodPost:
			return archive.RoleEditor
		case peppolPath == "/ping" && method == http.MethodPost:
			return archive.RoleEditor
		default:
			return archive.RoleEditor
		}
	}

	// Invoices.
	if path == "/invoices" || strings.HasPrefix(path, "/invoices/") {
		if method == http.MethodGet {
			return archive.RoleViewer
		}
		// POST ingest / validate / generate / preview — editor+.
		return archive.RoleEditor
	}

	// Default: admin (unknown routes stay locked down).
	return archive.RoleAdmin
}

func roleAtLeast(have, need string) bool {
	rank := map[string]int{
		archive.RoleViewer: 1,
		archive.RoleEditor: 2,
		archive.RoleAdmin:  3,
	}
	return rank[have] >= rank[need]
}
