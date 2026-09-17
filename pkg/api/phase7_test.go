package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/devthinker-ai/factura/pkg/api"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/session"
)

func addUser(t *testing.T, store *archive.Store, email, role, password string) archive.User {
	t.Helper()
	hash, err := session.FormatHash(password)
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.AddUser(email, "N", role, hash)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func jsonReq(method, path string, body any, token string) *http.Request {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func doJSON(t *testing.T, srv *api.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, jsonReq(method, path, body, token))
	return rr
}

func TestAuthStatus_Modes(t *testing.T) {
	srv, store := newTestServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/auth-status", nil, "")
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var st map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &st)
	if st["mode"] != "open" {
		t.Fatalf("mode=%s", st["mode"])
	}

	rr = doJSON(t, srv, http.MethodGet, "/health", nil, "")
	var health map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &health)
	if health["auth"] != "open" || health["status"] != "ok" {
		t.Fatalf("%v", health)
	}

	srv.Token = "op"
	rr = doJSON(t, srv, http.MethodGet, "/auth-status", nil, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &st)
	if st["mode"] != "token" {
		t.Fatalf("mode=%s", st["mode"])
	}

	addUser(t, store, "a@b.c", "admin", "password12")
	srv.BumpUsersCache(true)
	rr = doJSON(t, srv, http.MethodGet, "/auth-status", nil, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &st)
	if st["mode"] != "users" {
		t.Fatalf("mode=%s", st["mode"])
	}
}

func TestLogin_MFA_Recovery_Brute(t *testing.T) {
	srv, store := newTestServer(t)
	fixed := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	srv.SetNow(func() time.Time { return fixed })

	u := addUser(t, store, "mfa@test.de", "admin", "password12")
	srv.BumpUsersCache(true)

	// Unenrolled login.
	rr := doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var login map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	if login["token"] == nil || login["user"] == nil {
		t.Fatalf("%v", login)
	}

	secret, err := session.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	codes, err := session.GenerateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = session.HashRecoveryCode(c)
	}
	_ = store.UpsertTOTP(u.ID, secret, true, hashes, fixed.Format(time.RFC3339))

	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	if login["mfa_required"] != true {
		t.Fatalf("%v", login)
	}
	mfaTok := login["mfa_token"].(string)

	rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
		"mfa_token": mfaTok, "code": "000000",
	}, "")
	if rr.Code != 401 {
		t.Fatalf("wrong code: %d", rr.Code)
	}

	code, err := totp.GenerateCode(secret, fixed)
	if err != nil {
		t.Fatal(err)
	}
	rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
		"mfa_token": mfaTok, "code": code,
	}, "")
	if rr.Code != 200 {
		t.Fatalf("mfa ok: %d %s", rr.Code, rr.Body.String())
	}

	// Fresh login + recovery once.
	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	mfaTok = login["mfa_token"].(string)
	rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
		"mfa_token": mfaTok, "code": codes[0],
	}, "")
	if rr.Code != 200 {
		t.Fatalf("recovery: %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	mfaTok = login["mfa_token"].(string)
	rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
		"mfa_token": mfaTok, "code": codes[0],
	}, "")
	if rr.Code != 401 {
		t.Fatalf("reuse recovery: %d", rr.Code)
	}

	// 6th attempt → 429
	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	mfaTok = login["mfa_token"].(string)
	for i := 0; i < 5; i++ {
		rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
			"mfa_token": mfaTok, "code": "000000",
		}, "")
		if rr.Code != 401 {
			t.Fatalf("attempt %d: %d", i+1, rr.Code)
		}
	}
	rr = doJSON(t, srv, http.MethodPost, "/login/mfa", map[string]string{
		"mfa_token": mfaTok, "code": "000000",
	}, "")
	if rr.Code != 429 {
		t.Fatalf("want 429, got %d %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}

	// Login brute: 11th failed → 429
	srv2, store2 := newTestServer(t)
	addUser(t, store2, "brute@t.de", "admin", "password12")
	srv2.BumpUsersCache(true)
	for i := 0; i < 10; i++ {
		rr = doJSON(t, srv2, http.MethodPost, "/login", map[string]string{
			"email": "brute@t.de", "password": "wrong!!!!!!!",
		}, "")
		if rr.Code != 401 {
			t.Fatalf("fail %d: %d", i+1, rr.Code)
		}
	}
	rr = doJSON(t, srv2, http.MethodPost, "/login", map[string]string{
		"email": "brute@t.de", "password": "wrong!!!!!!!",
	}, "")
	if rr.Code != 429 {
		t.Fatalf("11th: %d", rr.Code)
	}

	// Clear TOTP → unenrolled body shape again.
	_ = store.ClearTOTP(u.ID)
	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "mfa@test.de", "password": "password12",
	}, "")
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	login = map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	if login["mfa_required"] != nil {
		t.Fatalf("should be plain login: %v", login)
	}
	if login["token"] == nil || login["user"] == nil {
		t.Fatalf("missing token/user: %v", login)
	}
}

func TestZeroUsers_OpenAndToken(t *testing.T) {
	srv, _ := newTestServer(t)
	body := readTD(t, "xrechnung-302-cii.xml")

	req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("open: %d", rr.Code)
	}

	srv.Token = "secret-token"
	req = httptest.NewRequest(http.MethodGet, "/invoices", nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), `"unauthorized"`) {
		t.Fatalf("token mode unauthorized: %d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/invoices", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("operator: %d", rr.Code)
	}
}

func TestRoleMatrix(t *testing.T) {
	srv, store := newTestServer(t)
	admin := addUser(t, store, "admin@t.de", "admin", "password12")
	editor := addUser(t, store, "editor@t.de", "editor", "password12")
	viewer := addUser(t, store, "viewer@t.de", "viewer", "password12")
	srv.BumpUsersCache(true)

	issue := func(u archive.User) string {
		tok, err := srv.Sessions.Issue(session.User{ID: u.ID, Email: u.Email, Role: u.Role})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	adminTok := issue(admin)
	editorTok := issue(editor)
	viewerTok := issue(viewer)

	type cell struct {
		role   string
		method string
		path   string
		want   int // 401 forbidden or 2xx/other non-forbidden
	}
	// Spot-check matrix corners + machine-key bypass covered separately.
	cases := []struct {
		tok    string
		method string
		path   string
		want   int
		err    string
	}{
		{viewerTok, "POST", "/invoices", 401, "forbidden"},
		{viewerTok, "GET", "/invoices", 200, ""},
		{editorTok, "POST", "/license", 401, "forbidden"},
		{editorTok, "GET", "/license", 200, ""},
		{editorTok, "POST", "/companies", 422, ""}, // role OK; body invalid → not forbidden
		{adminTok, "GET", "/users", 200, ""},
		{viewerTok, "GET", "/users", 401, "forbidden"},
		{viewerTok, "POST", "/2fa/setup", 200, ""},
	}
	for _, c := range cases {
		var body any
		if c.method == "POST" || c.method == "PATCH" {
			body = map[string]any{}
		}
		if c.path == "/companies" {
			body = map[string]string{"name": ""}
		}
		rr := doJSON(t, srv, c.method, c.path, body, c.tok)
		if rr.Code != c.want {
			t.Errorf("%s %s: want %d got %d %s", c.method, c.path, c.want, rr.Code, rr.Body.String())
			continue
		}
		if c.err != "" && !strings.Contains(rr.Body.String(), c.err) {
			t.Errorf("%s %s: want error %q in %s", c.method, c.path, c.err, rr.Body.String())
		}
	}

	// Machine key bypasses matrix.
	key, err := store.CreateKey("embed", 30, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// Reload validator cache
	srv2 := api.New(store)
	srv2.BumpUsersCache(true)
	rr := doJSON(t, srv2, http.MethodPost, "/invoices/validate", map[string]string{}, key.ID)
	// May be 400/415 etc — must NOT be forbidden
	if rr.Code == 401 && strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("machine key hit role matrix: %s", rr.Body.String())
	}
}

func TestUsersCRUD_Bootstrap(t *testing.T) {
	srv, _ := newTestServer(t)

	rr := doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "chef@kanzlei.de", "name": "Chef", "role": "editor", "password": "password12",
	}, "")
	if rr.Code != 422 {
		t.Fatalf("first non-admin: %d %s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "chef@kanzlei.de", "name": "Chef", "role": "admin", "password": "password12",
	}, "")
	if rr.Code != 201 {
		t.Fatalf("bootstrap: %d %s", rr.Code, rr.Body.String())
	}
	srv.BumpUsersCache(true)

	rr = doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "sec@kanzlei.de", "role": "viewer", "password": "password12",
	}, "")
	if rr.Code != 401 {
		t.Fatalf("second public: %d", rr.Code)
	}

	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "chef@kanzlei.de", "password": "password12",
	}, "")
	var login map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	tok := login["token"].(string)
	user := login["user"].(map[string]any)
	adminID := user["id"].(string)

	rr = doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "sec@kanzlei.de", "role": "viewer", "password": "password12",
	}, tok)
	if rr.Code != 201 {
		t.Fatalf("admin create: %d %s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "sec@kanzlei.de", "role": "viewer", "password": "password12",
	}, tok)
	if rr.Code != 409 {
		t.Fatalf("dup: %d", rr.Code)
	}

	rr = doJSON(t, srv, http.MethodPost, "/users", map[string]string{
		"email": "short@t.de", "role": "viewer", "password": "short",
	}, tok)
	if rr.Code != 422 {
		t.Fatalf("short pw: %d", rr.Code)
	}

	rr = doJSON(t, srv, http.MethodPatch, "/users/"+adminID, map[string]string{"role": "viewer"}, tok)
	if rr.Code != 422 {
		t.Fatalf("self-demote: %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, srv, http.MethodDelete, "/users/"+adminID, nil, tok)
	if rr.Code != 422 {
		t.Fatalf("self-delete: %d", rr.Code)
	}

	rr = doJSON(t, srv, http.MethodGet, "/me", nil, tok)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	rr = doJSON(t, srv, http.MethodGet, "/me", nil, "")
	if rr.Code != 401 {
		t.Fatal(rr.Code)
	}
	rr = doJSON(t, srv, http.MethodGet, "/me", nil, "not-a-session")
	if rr.Code != 401 {
		t.Fatal(rr.Code)
	}
}

func Test2FALifecycle(t *testing.T) {
	srv, store := newTestServer(t)
	fixed := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	srv.SetNow(func() time.Time { return fixed })
	u := addUser(t, store, "2fa@t.de", "editor", "password12")
	srv.BumpUsersCache(true)
	tok, _ := srv.Sessions.Issue(session.User{ID: u.ID, Email: u.Email, Role: u.Role})

	rr := doJSON(t, srv, http.MethodPost, "/2fa/setup", map[string]any{}, tok)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var setup map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &setup)
	if !strings.HasPrefix(setup["qr"], "data:image/png") {
		t.Fatalf("qr=%s", setup["qr"])
	}
	code, err := totp.GenerateCode(setup["secret"], fixed)
	if err != nil {
		t.Fatal(err)
	}
	rr = doJSON(t, srv, http.MethodPost, "/2fa/confirm", map[string]string{"code": code}, tok)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var conf map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &conf)
	codes, _ := conf["recovery_codes"].([]any)
	if len(codes) != 10 {
		t.Fatalf("codes=%d", len(codes))
	}

	// Re-confirm does not re-issue.
	rr = doJSON(t, srv, http.MethodPost, "/2fa/confirm", map[string]string{"code": code}, tok)
	_ = json.Unmarshal(rr.Body.Bytes(), &conf)
	codes2, _ := conf["recovery_codes"].([]any)
	if len(codes2) != 0 {
		t.Fatalf("re-confirm re-issued: %v", codes2)
	}

	rr = doJSON(t, srv, http.MethodDelete, "/2fa", nil, tok)
	if rr.Code != 204 {
		t.Fatal(rr.Code)
	}
	rr = doJSON(t, srv, http.MethodDelete, "/2fa", nil, tok)
	if rr.Code != 204 {
		t.Fatal("idempotent delete")
	}

	rr = doJSON(t, srv, http.MethodPost, "/login", map[string]string{
		"email": "2fa@t.de", "password": "password12",
	}, "")
	var login map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	if login["mfa_required"] != nil {
		t.Fatalf("plain again: %v", login)
	}
}
