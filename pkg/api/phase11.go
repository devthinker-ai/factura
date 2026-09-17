package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/generate"
)

func (s *Server) mountPhase11(r chi.Router) {
	r.Get("/companies/{id}", s.handleCompaniesGet)
}

func (s *Server) handleCompaniesGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	c, err := s.Store.CompanyByID(id)
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	b, err := s.Store.GetCompanyBranding(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, companyWithBranding(c, b, true))
}

// handleCompaniesPatch: body {} → set-default (compat); {"branding":{…}} → update branding.
// Both branding and any other key in one body ⇒ 422 (one action per request).
func (s *Server) handleCompaniesPatch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	raw := map[string]json.RawMessage{}
	trimmed := strings.TrimSpace(string(data))
	if trimmed != "" && trimmed != "{}" {
		if err := json.Unmarshal(data, &raw); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
			return
		}
	}
	_, hasBranding := raw["branding"]
	if hasBranding {
		if len(raw) > 1 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "one action per request",
			})
			return
		}
		var body struct {
			Branding *archive.CompanyBranding `json:"branding"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
			return
		}
		normalized, err := validateAndNormalizeBranding(body.Branding)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error":  "invalid branding",
				"detail": err.Error(),
			})
			return
		}
		if err := s.Store.SetCompanyBranding(id, normalized); err != nil {
			if errors.Is(err, archive.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		c, err := s.Store.CompanyByID(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, companyWithBranding(c, normalized, true))
		return
	}

	// Set-default (body {} / no branding key) — existing behavior.
	if err := s.Store.SetDefaultCompany(id); err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	c, err := s.Store.CompanyByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	b, _ := s.Store.GetCompanyBranding(id)
	writeJSON(w, http.StatusOK, companyWithBranding(c, b, true))
}

func (s *Server) handleCompaniesList(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Store.ListCompanies()
	if err != nil {
		s.log.Printf("companies: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		b, err := s.Store.GetCompanyBranding(c.ID)
		if err != nil {
			b = &archive.CompanyBranding{}
		}
		out = append(out, companyWithBranding(c, b, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"companies": out})
}

func companyWithBranding(c archive.Company, b *archive.CompanyBranding, includeLogo bool) map[string]any {
	if b == nil {
		b = &archive.CompanyBranding{}
	}
	branding := map[string]any{}
	if b.Accent != "" {
		branding["accent"] = b.Accent
	}
	if b.HeaderText != "" {
		branding["header_text"] = b.HeaderText
	}
	if b.FooterText != "" {
		branding["footer_text"] = b.FooterText
	}
	hasLogo := b.LogoB64 != ""
	if includeLogo && hasLogo {
		branding["logo_b64"] = b.LogoB64
		branding["logo_mime"] = b.LogoMime
		if branding["logo_mime"] == "" {
			branding["logo_mime"] = "image/jpeg"
		}
	} else if hasLogo {
		branding["logo_mime"] = "image/jpeg"
	}
	return map[string]any{
		"id":            c.ID,
		"name":          c.Name,
		"seller_vat_id": c.SellerVATID,
		"peppol_id":     c.PeppolID,
		"is_default":    c.IsDefault,
		"created_at":    c.CreatedAt,
		"branding":      branding,
		"has_logo":      hasLogo,
	}
}

func validateAndNormalizeBranding(b *archive.CompanyBranding) (*archive.CompanyBranding, error) {
	if b == nil {
		return &archive.CompanyBranding{}, nil
	}
	out := &archive.CompanyBranding{
		Accent:     strings.TrimSpace(b.Accent),
		HeaderText: b.HeaderText,
		FooterText: b.FooterText,
	}
	if utf8.RuneCountInString(out.HeaderText) > 500 {
		return nil, errors.New("header_text exceeds 500 characters")
	}
	if utf8.RuneCountInString(out.FooterText) > 500 {
		return nil, errors.New("footer_text exceeds 500 characters")
	}
	if out.Accent != "" {
		if _, ok, err := generate.ParseAccent(out.Accent); err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("accent must be #RRGGBB")
		}
	}
	logoB64 := strings.TrimSpace(b.LogoB64)
	if logoB64 != "" {
		mime := strings.ToLower(strings.TrimSpace(b.LogoMime))
		if mime != "image/png" && mime != "image/jpeg" && mime != "image/jpg" {
			return nil, errors.New("logo_mime must be image/png or image/jpeg")
		}
		raw, err := base64.StdEncoding.DecodeString(logoB64)
		if err != nil {
			// try raw URL-safe / no padding
			raw, err = base64.RawStdEncoding.DecodeString(logoB64)
			if err != nil {
				return nil, errors.New("logo_b64 is not valid base64")
			}
		}
		jpegBytes, err := generate.NormalizeLogo(raw, mime)
		if err != nil {
			return nil, err
		}
		out.LogoB64 = base64.StdEncoding.EncodeToString(jpegBytes)
		out.LogoMime = "image/jpeg"
	}
	return out, nil
}

func templateFromBranding(b *archive.CompanyBranding) (generate.Template, error) {
	var t generate.Template
	if b == nil {
		return t, nil
	}
	t.HeaderText = b.HeaderText
	t.FooterText = b.FooterText
	if b.Accent != "" {
		rgb, ok, err := generate.ParseAccent(b.Accent)
		if err != nil {
			return t, err
		}
		if ok {
			t.Accent = rgb
			t.HasAccent = true
		}
	}
	if b.LogoB64 != "" {
		raw, err := base64.StdEncoding.DecodeString(b.LogoB64)
		if err != nil {
			return t, err
		}
		t.Logo = raw
		t.HasLogo = true
	}
	return t, nil
}

func (s *Server) resolveGenerateTemplate(r *http.Request) (generate.Template, int, error) {
	q := r.URL.Query().Get("company_id")
	if q != "" {
		id, err := strconv.ParseInt(q, 10, 64)
		if err != nil || id < 1 {
			return generate.Template{}, http.StatusUnprocessableEntity, errors.New("company_id must be a positive integer")
		}
		b, err := s.Store.GetCompanyBranding(id)
		if err != nil {
			if errors.Is(err, archive.ErrNotFound) {
				return generate.Template{}, http.StatusNotFound, errors.New("not found")
			}
			return generate.Template{}, http.StatusInternalServerError, err
		}
		t, err := templateFromBranding(b)
		if err != nil {
			return generate.Template{}, http.StatusUnprocessableEntity, err
		}
		return t, 0, nil
	}
	c, err := s.Store.DefaultCompany()
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			return generate.Template{}, 0, nil // Solo / no company → zero template
		}
		return generate.Template{}, http.StatusInternalServerError, err
	}
	b, err := s.Store.GetCompanyBranding(c.ID)
	if err != nil {
		return generate.Template{}, 0, nil
	}
	t, err := templateFromBranding(b)
	if err != nil {
		return generate.Template{}, 0, nil
	}
	return t, 0, nil
}
