// Package api is a thin HTTP wrapper over Phase 1 packages.
// Zero business logic here — parse/validate/generate/archive come from pkg/*.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/apikeys"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/generate"
	"github.com/devthinker-ai/factura/pkg/license"
	"github.com/devthinker-ai/factura/pkg/model"
	"github.com/devthinker-ai/factura/pkg/parse"
	"github.com/devthinker-ai/factura/pkg/peppol"
	"github.com/devthinker-ai/factura/pkg/session"
	"github.com/devthinker-ai/factura/pkg/validate"
	"github.com/devthinker-ai/factura/pkg/web"
)

const maxBodyBytes = 10 << 20 // 10 MB

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	Store    *archive.Store
	Token    string // empty = auth off (when no users)
	Limit    int    // req/min when auth on; 0 → 60
	Version  string // binary version (ldflags); default "dev"
	Peppol   *peppol.Engine
	Keys     *apikeys.Validator
	Caps     license.Caps
	Sessions *session.Manager
	log      *log.Logger

	mu      sync.Mutex
	bucket  float64
	lastRef time.Time
	rateN   float64 // tokens per second
	burst   float64

	capsMu sync.RWMutex
	now    func() time.Time

	mfaGate  *session.MFAGate
	loginLim *session.LoginLimiter

	// usersExist cache — bumped on AddUser/RemoveUser; else 30 s TTL.
	usersMu      sync.Mutex
	usersExists  bool
	usersChecked time.Time
	usersCached  bool

	// spa is the /app/ handler; nil → pkg/web production embed.
	spa http.Handler

	pollCancel context.CancelFunc
}

// New builds a Server. Token from FACTURA_TOKEN; rate from FACTURA_RATE_LIMIT.
// License resolves fail-open from env / settings store.
func New(store *archive.Store) *Server {
	s := &Server{
		Store:   store,
		Token:   os.Getenv("FACTURA_TOKEN"),
		Version: "dev",
		log:     log.Default(),
		now:     func() time.Time { return time.Now().UTC() },
	}
	s.Limit = 60
	if v := os.Getenv("FACTURA_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.Limit = n
		}
	}
	s.rateN = float64(s.Limit) / 60.0
	s.burst = float64(s.Limit)
	s.bucket = s.burst
	s.lastRef = time.Now()
	s.Caps = license.Resolve(nil, "", store)
	if v, err := apikeys.NewValidator(store); err == nil {
		s.Keys = v
	}
	if mgr, err := session.NewManager(store); err == nil {
		s.Sessions = mgr
	}
	s.mfaGate = session.NewMFAGate(s.now)
	s.loginLim = session.NewLoginLimiter(s.now)
	return s
}

// SetNow injects a clock for metering / quota / TOTP tests.
func (s *Server) SetNow(fn func() time.Time) {
	if fn == nil {
		return
	}
	s.now = fn
	if s.Keys != nil {
		s.Keys.SetNow(fn)
	}
	if s.Sessions != nil {
		s.Sessions.SetNow(fn)
	}
	if s.mfaGate != nil {
		s.mfaGate.SetNow(fn)
	}
	if s.loginLim != nil {
		s.loginLim.SetNow(fn)
	}
}

// ApplyCaps replaces live caps (tests / POST /license).
func (s *Server) ApplyCaps(c license.Caps) {
	s.setCaps(c)
}

// SetSPA overrides the embedded SPA handler (tests).
func (s *Server) SetSPA(h http.Handler) {
	s.spa = h
}

// Handler returns the chi router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.recoverer)
	r.Use(s.requestLogger)
	r.Get("/health", s.handleHealth)
	r.Get("/", s.handleRoot)

	spa := s.spa
	if spa == nil {
		spa = web.Handler()
	}
	// Strip /app so the SPA FS sees /, /assets/*, /invoices/:id, etc.
	stripped := http.StripPrefix("/app", spa)
	r.Get("/app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusFound)
	})
	r.Handle("/app/*", stripped)

	// Phase 7: public auth endpoints (login, auth-status, first-user POST /users).
	s.mountAuth(r)

	r.Group(func(r chi.Router) {
		r.Use(s.auth)
		r.Use(s.rateLimit)
		r.Post("/invoices", s.handleIngest)
		r.Post("/invoices/validate", s.handleValidate)
		r.Get("/invoices", s.handleList)
		r.Get("/invoices/vendors", s.handleVendors)
		r.Get("/invoices/export", s.handleExport)
		r.Post("/invoices/generate", s.handleGenerate(http.StatusCreated))
		r.Post("/invoices/generate/preview", s.handleGenerate(http.StatusOK))
		r.Get("/invoices/{id}", s.handleGet)
		r.Get("/invoices/{id}/original", s.handleOriginal)
		r.Get("/invoices/{id}/evidence", s.handleEvidence)
		r.Get("/audit", s.handleAudit)
		s.mountPhase6(r)
		s.mountPhase11(r)
		s.mountPeppol(r)
		s.mountAuthAuthed(r)
	})
	return r
}

// StartPeppolPoller runs AP poll → ingest when FACTURA_PEPPOL_POLL=1.
func (s *Server) StartPeppolPoller(parent context.Context) {
	if os.Getenv("FACTURA_PEPPOL_POLL") != "1" || s.Peppol == nil {
		return
	}
	interval := 60 * time.Second
	if v := os.Getenv("FACTURA_PEPPOL_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	ctx, cancel := context.WithCancel(parent)
	s.pollCancel = cancel
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.Peppol.PollAndIngest(ctx, time.Time{}); err != nil {
					s.log.Printf("peppol poller: %v", err)
				}
			}
		}
	}()
}

// StopPeppolPoller stops the background poller.
func (s *Server) StopPeppolPoller() {
	if s.pollCancel != nil {
		s.pollCancel()
	}
}

const rootRedirectHTML = `<!DOCTYPE html><html><head><meta charset="utf-8">` +
	`<meta http-equiv="refresh" content="0;url=/app/">` +
	`<title>Factura</title></head><body><a href="/app/">Open Factura console</a></body></html>`

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Location", "/app/")
	w.WriteHeader(http.StatusFound)
	_, _ = io.WriteString(w, rootRedirectHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ver := s.Version
	if ver == "" {
		ver = "dev"
	}
	caps := s.getCaps()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": ver,
		"plan":    caps.Plan,
		"auth":    s.authMode(),
	})
}

func (s *Server) handleVendors(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	vendors, err := s.Store.DistinctVendors(limit)
	if err != nil {
		s.log.Printf("vendors: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vendors": vendors})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if !s.checkKeyQuota(w, r) {
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return // already responded (413)
	}
	extID := r.Header.Get("X-Factura-External-ID")
	source := r.Header.Get("X-Factura-Source")

	if extID != "" {
		if existing, err := s.Store.GetByExternalID(extID); err == nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "external_id already exists",
				"id":    existing.ID,
			})
			return
		}
	}

	row, err := s.Store.IngestWithSource(data, extID, source)
	if err != nil {
		if errors.Is(err, archive.ErrDuplicate) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "external_id already exists",
				"id":    row.ID,
			})
			return
		}
		s.log.Printf("ingest error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	if row.Status == archive.StatusParseError {
		detail := "unrecognized invoice format"
		var rep validate.Report
		if len(row.Report) > 0 {
			_ = json.Unmarshal(row.Report, &rep)
			if rep.Summary != "" {
				detail = rep.Summary
			}
			if len(rep.Violations) > 0 && rep.Violations[0].Human != "" {
				detail = rep.Violations[0].Human
			}
		}
		// Prefer typed parse reason when present in summary.
		if pe := parseReason(detail); pe != "" {
			detail = pe
		}
		s.meterInvoiceOp(r)
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"error":  "unrecognized invoice format",
			"detail": detail,
			"id":     row.ID,
		})
		return
	}

	s.meterInvoiceOp(r)
	writeJSON(w, http.StatusCreated, ingestResponse(row))
}

func ingestResponse(row archive.Row) map[string]any {
	var rep any
	if len(row.Report) > 0 {
		_ = json.Unmarshal(row.Report, &rep)
	}
	return map[string]any{
		"id":             row.ID,
		"external_id":    row.ExternalID,
		"status":         row.Status,
		"format":         row.Format,
		"report":         rep,
		"vendor_name":    row.VendorName,
		"invoice_number": row.InvoiceNumber,
		"invoice_type":   row.InvoiceType,
	}
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	if parse.DetectFormat(data) == parse.FormatUnknown {
		_, _, perr := parse.Parse(data)
		detail := "unrecognized invoice format"
		if perr != nil {
			var pe *parse.Error
			if errors.As(perr, &pe) {
				detail = pe.Reason
			} else {
				detail = perr.Error()
			}
		}
		rep := validate.ValidateBytes(data)
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"report": rep,
			"detail": detail,
		})
		return
	}
	rep := validate.ValidateBytes(data)
	writeJSON(w, http.StatusOK, map[string]any{"report": rep})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	f := filterFromQuery(r)
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	total, err := s.Store.Count(f)
	if err != nil {
		s.log.Printf("count: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	f.Limit = limit
	f.Offset = offset
	rows, err := s.Store.List(f)
	if err != nil {
		s.log.Printf("list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if rows == nil {
		rows = []archive.Row{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "total": total})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	row, err := s.Store.Get(id)
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		s.log.Printf("get: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleOriginal(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	row, err := s.Store.Get(id)
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	blob, err := s.Store.Original(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	ct, ext := contentTypeForFormat(row.Format)
	name := row.InvoiceNumber
	if name == "" {
		name = fmt.Sprintf("invoice-%d", id)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-original.%s"`, sanitizeName(name), ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	f := filterFromQuery(r)
	ts := time.Now().UTC().Format("20060102-150405")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="factura-export-%s.csv"`, ts))
	w.WriteHeader(http.StatusOK)
	if err := s.Store.Export(f, w); err != nil {
		s.log.Printf("export: %v", err)
	}
}

func (s *Server) handleGenerate(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkKeyQuota(w, r) {
			return
		}
		data, err := readBodyLimited(w, r)
		if err != nil {
			return
		}
		inv, err := model.FromJSON(data)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		tmpl, statusCode, err := s.resolveGenerateTemplate(r)
		if err != nil {
			code := statusCode
			if code == 0 {
				code = http.StatusInternalServerError
			}
			msg := err.Error()
			if code == http.StatusNotFound {
				msg = "not found"
			}
			writeJSON(w, code, map[string]string{"error": msg})
			return
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "both"
		}
		download := r.URL.Query().Get("download") == "1"

		dir, err := os.MkdirTemp("", "factura-gen-*")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		defer os.RemoveAll(dir)

		xrPath, zfPath, err := generate.GenerateWith(*inv, dir, &tmpl)
		if err != nil {
			detail := err.Error()
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error":  err.Error(),
				"detail": detail,
			})
			return
		}

		xrBytes, err := os.ReadFile(xrPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		zfBytes, err := os.ReadFile(zfPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		xrName := filepath.Base(xrPath)
		zfName := filepath.Base(zfPath)

		// Meter generate once on success (preview counts too — it's an invoice op).
		s.meterInvoiceOp(r)

		if download {
			switch format {
			case "xrechnung":
				w.Header().Set("Content-Type", "application/xml")
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, xrName))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(xrBytes)
				return
			case "zugferd":
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zfName))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(zfBytes)
				return
			default:
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", `attachment; filename="factura-invoice.zip"`)
				w.WriteHeader(http.StatusOK)
				if err := writeZip(w, map[string][]byte{xrName: xrBytes, zfName: zfBytes}); err != nil {
					s.log.Printf("zip: %v", err)
				}
				return
			}
		}

		body := map[string]any{
			"self_check":         true,
			"filename_xrechnung": xrName,
			"filename_zugferd":   zfName,
		}
		switch format {
		case "xrechnung":
			body["xrechnung"] = string(xrBytes)
			body["zugferd_pdf_base64"] = ""
		case "zugferd":
			body["xrechnung"] = ""
			body["zugferd_pdf_base64"] = base64.StdEncoding.EncodeToString(zfBytes)
		default:
			body["xrechnung"] = string(xrBytes)
			body["zugferd_pdf_base64"] = base64.StdEncoding.EncodeToString(zfBytes)
		}
		writeJSON(w, status, body)
	}
}

// --- middleware ---

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		const prefix = "Bearer "
		token := ""
		if strings.HasPrefix(h, prefix) {
			token = strings.TrimPrefix(h, prefix)
		}

		// Step 1 — machine keys FIRST (unchanged).
		if apikeys.IsAPIKeyToken(token) {
			if s.Keys == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			cached, result, retryAfter := s.Keys.Check(token, false)
			switch result {
			case apikeys.CheckUnauthorized:
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			case apikeys.CheckRateLimited:
				apikeys.WriteRateLimited(w, retryAfter)
				return
			case apikeys.CheckQuotaExceeded:
				apikeys.WriteQuotaExceeded(w)
				return
			}
			ctx := apikeys.WithKey(r.Context(), cached)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Step 2 — zero users → today's exact path (byte-identical).
		if !s.usersExist() {
			if s.Token == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !strings.HasPrefix(h, prefix) ||
				subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Step 3 — users exist.
		// 3a. Operator token bypasses the role matrix (legacy admin surface).
		if s.Token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		// 3b. Session JWT → role check → next / 401 forbidden.
		if s.Sessions != nil && token != "" {
			claims, err := s.Sessions.Parse(token)
			if err == nil {
				u, uerr := s.Store.GetUser(claims.Subject)
				if uerr != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
					return
				}
				need := authRoleFor(r.Method, r.URL.Path)
				if !roleAtLeast(u.Role, need) {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "forbidden"})
					return
				}
				ctx := session.WithUser(r.Context(), session.AuthUser{
					ID: u.ID, Email: u.Email, Role: u.Role, Name: u.Name,
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		// 3c. No/bad token → today's unauthorized body.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per-key RPM is enforced in auth; operator bucket is operator-token only.
		if _, ok := apikeys.KeyFromContext(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		// Rate limit ONLY when auth is on (spec).
		if s.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.takeToken() {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) takeToken() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(s.lastRef).Seconds()
	s.lastRef = now
	s.bucket += elapsed * s.rateN
	if s.bucket > s.burst {
		s.bucket = s.burst
	}
	if s.bucket < 1 {
		return false
	}
	s.bucket--
	return true
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Printf("panic: %v", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		s.log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	})
}

// --- helpers ---

func readBodyLimited(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	// Reject oversized Content-Length before reading.
	if r.ContentLength > maxBodyBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": "body too large",
			"limit": maxBodyBytes,
		})
		return nil, errors.New("413")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(err.Error(), "request body too large") {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
				"error": "body too large",
				"limit": maxBodyBytes,
			})
			return nil, errors.New("413")
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return nil, err
	}
	return data, nil
}

func filterFromQuery(r *http.Request) archive.Filter {
	q := r.URL.Query()
	return archive.Filter{
		Status: q.Get("status"),
		Vendor: q.Get("vendor"),
		Query:  q.Get("q"),
		Since:  q.Get("since"),
		Until:  q.Get("until"),
	}
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func contentTypeForFormat(format string) (ct, ext string) {
	switch parse.Format(format) {
	case parse.FormatZUGFeRDPDF, parse.FormatFacturXPDF:
		return "application/pdf", "pdf"
	default:
		return "application/xml", "xml"
	}
}

func sanitizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
	if s == "" {
		return "invoice"
	}
	return s
}

func parseReason(s string) string {
	const p = "parse: "
	if strings.HasPrefix(s, p) {
		rest := strings.TrimPrefix(s, p)
		if i := strings.Index(rest, ":"); i > 0 {
			return strings.TrimSpace(rest[:i])
		}
		return rest
	}
	return ""
}

// ListenAndServe runs the HTTP server with graceful shutdown.
func ListenAndServe(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
