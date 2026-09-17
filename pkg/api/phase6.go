package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/apikeys"
	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/license"
)

func (s *Server) mountPhase6(r chi.Router) {
	r.Get("/license", s.handleLicenseGet)
	r.Post("/license", s.handleLicensePost)
	r.Delete("/license", s.handleLicenseDelete)
	r.Get("/companies", s.handleCompaniesList)
	r.Post("/companies", s.handleCompaniesAdd)
	r.Patch("/companies/{id}", s.handleCompaniesPatch)
	r.Get("/api-keys", s.handleAPIKeysList)
	r.Post("/api-keys", s.handleAPIKeysCreate)
	r.Patch("/api-keys/{id}", s.handleAPIKeysPatch)
	r.Delete("/api-keys/{id}", s.handleAPIKeysDelete)
	r.Get("/usage", s.handleUsage)
}

func (s *Server) getCaps() license.Caps {
	s.capsMu.RLock()
	defer s.capsMu.RUnlock()
	return s.Caps
}

func (s *Server) setCaps(c license.Caps) {
	s.capsMu.Lock()
	s.Caps = c
	s.capsMu.Unlock()
}

func (s *Server) handleLicenseGet(w http.ResponseWriter, _ *http.Request) {
	caps := s.getCaps()
	companies, _ := s.Store.ListCompanies()
	keys, _ := s.Store.ListKeys()
	_, invCount := s.Store.OperatorUsage(s.now)
	writeJSON(w, http.StatusOK, licenseResponse(caps, len(companies), len(keys), invCount))
}

func licenseResponse(caps license.Caps, companies, apiKeys, invoices int) map[string]any {
	exp := ""
	if !caps.ExpiresAt.IsZero() {
		exp = caps.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"plan":     caps.Plan,
		"licensed": caps.Licensed,
		"subject":  caps.Subject,
		"expires":  exp,
		"caps": map[string]any{
			"max_companies": caps.MaxCompanies,
			"max_api_keys":  caps.MaxAPIKeys,
			"max_clients":   caps.MaxClients,
			"peppol":        caps.Peppol,
		},
		"in_use": map[string]any{
			"companies":           companies,
			"api_keys":            apiKeys,
			"invoices_this_month": invoices,
		},
	}
}

func (s *Server) handleLicensePost(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &body); err != nil || strings.TrimSpace(body.Key) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  "invalid_license_key",
			"detail": "key required",
		})
		return
	}
	claims, err := license.Validate(body.Key)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  "invalid_license_key",
			"detail": err.Error(),
		})
		return
	}
	if err := s.Store.SetSetting("license_key", strings.TrimSpace(body.Key)); err != nil {
		s.log.Printf("license persist: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	caps := license.CapsFromClaims(claims)
	s.setCaps(caps)
	companies, _ := s.Store.ListCompanies()
	keys, _ := s.Store.ListKeys()
	_, invCount := s.Store.OperatorUsage(s.now)
	writeJSON(w, http.StatusOK, licenseResponse(caps, len(companies), len(keys), invCount))
}

func (s *Server) handleLicenseDelete(w http.ResponseWriter, _ *http.Request) {
	if err := s.Store.DeleteSetting("license_key"); err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		s.log.Printf("license delete: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.setCaps(license.FreeCaps())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCompaniesAdd(w http.ResponseWriter, r *http.Request) {
	caps := s.getCaps()
	existing, err := s.Store.ListCompanies()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if len(existing) >= caps.MaxCompanies {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":  "plan_limit",
			"detail": planCompaniesDetail(caps),
		})
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Name        string `json:"name"`
		SellerVATID string `json:"seller_vat_id"`
		PeppolID    string `json:"peppol_id"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "name required"})
		return
	}
	c, err := s.Store.AddCompany(body.Name, body.SellerVATID, body.PeppolID)
	if err != nil {
		if errors.Is(err, archive.ErrDuplicateVAT) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate seller_vat_id"})
			return
		}
		s.log.Printf("add company: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func planCompaniesDetail(caps license.Caps) string {
	switch caps.Plan {
	case license.PlanPro:
		return "Pro allows 10 companies"
	case license.PlanKanzlei:
		return "Kanzlei allows 50 companies"
	default:
		return "Solo allows 1 company"
	}
}

func planAPIKeysDetail(caps license.Caps) string {
	switch caps.Plan {
	case license.PlanPro:
		return "Pro allows 20 API keys"
	case license.PlanKanzlei:
		return "Kanzlei allows 50 API keys"
	default:
		return "Solo allows 2 API keys"
	}
}

func (s *Server) handleAPIKeysList(w http.ResponseWriter, _ *http.Request) {
	keys, err := s.Store.ListKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	period := s.now().Format("2006-01")
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		used := k.InvoicesThisMonth
		if k.Period != period {
			used = 0
		}
		out = append(out, map[string]any{
			"id":                  k.ID,
			"name":                k.Name,
			"rpm":                 k.RPM,
			"monthly_invoices":    k.MonthlyInvoices,
			"invoices_this_month": used,
			"enabled":             k.Enabled,
			"created_at":          k.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (s *Server) handleAPIKeysCreate(w http.ResponseWriter, r *http.Request) {
	caps := s.getCaps()
	keys, err := s.Store.ListKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if len(keys) >= caps.MaxAPIKeys {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":  "plan_limit",
			"detail": planAPIKeysDetail(caps),
		})
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Name            string `json:"name"`
		RPM             int    `json:"rpm"`
		MonthlyInvoices int    `json:"monthly_invoices"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "name required"})
		return
	}
	k, err := s.Store.CreateKey(body.Name, body.RPM, body.MonthlyInvoices)
	if err != nil {
		s.log.Printf("create key: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if s.Keys != nil {
		_ = s.Keys.Refresh()
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":              k.ID,
		"name":             k.Name,
		"rpm":              k.RPM,
		"monthly_invoices": k.MonthlyInvoices,
	})
}

func (s *Server) handleAPIKeysPatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &body); err != nil || body.Enabled == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "enabled required"})
		return
	}
	if err := s.Store.SetKeyEnabled(id, *body.Enabled); err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if s.Keys != nil {
		_ = s.Keys.Refresh()
	}
	k, err := s.Store.KeyByID(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": *body.Enabled})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      k.ID,
		"name":    k.Name,
		"enabled": k.Enabled,
	})
}

func (s *Server) handleAPIKeysDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.Store.DeleteKey(id); err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if s.Keys != nil {
		_ = s.Keys.Refresh()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	sum, err := s.Store.UsageSummary(s.now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if k, ok := apikeys.KeyFromContext(r.Context()); ok {
		filtered := make([]archive.KeyUsage, 0, 1)
		for _, row := range sum.Keys {
			if row.ID == k.ID {
				filtered = append(filtered, row)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"period":              sum.Period,
			"invoices_this_month": 0,
			"keys":                filtered,
		})
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// meterInvoiceOp records one invoice operation for API-key traffic only.
// Operator / no-auth traffic is never metered (dashboard operator counter is separate).
func (s *Server) meterInvoiceOp(r *http.Request) {
	k, ok := apikeys.KeyFromContext(r.Context())
	if !ok {
		return
	}
	if s.Keys != nil {
		if err := s.Keys.RecordUsage(k.ID); err != nil {
			s.log.Printf("meter key: %v", err)
		}
	}
}

// checkKeyQuota returns false (and writes 403) when an API-key caller is over quota.
func (s *Server) checkKeyQuota(w http.ResponseWriter, r *http.Request) bool {
	k, ok := apikeys.KeyFromContext(r.Context())
	if !ok {
		return true
	}
	period := s.now().Format("2006-01")
	used := k.InvoicesThisMonth
	if k.Period != period {
		used = 0
	}
	if k.MonthlyInvoices > 0 && used >= k.MonthlyInvoices {
		apikeys.WriteQuotaExceeded(w)
		return false
	}
	return true
}
