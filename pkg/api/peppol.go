package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/archive"
	"github.com/devthinker-ai/factura/pkg/peppol"
)

// http.StatusFailedDependency = 424 (AP rejection only).
const statusFailedDependency = 424

// SetPeppol wires the Peppol engine (optional — nil disables Peppol routes).
func (s *Server) SetPeppol(eng *peppol.Engine) {
	s.Peppol = eng
}

func (s *Server) mountPeppol(r chi.Router) {
	r.Route("/peppol", func(r chi.Router) {
		r.Use(s.requirePeppolPlan)
		r.Post("/inbound", s.handlePeppolInboundPost)
		r.Get("/inbound", s.handlePeppolInboundGet)
		r.Post("/send", s.handlePeppolSend)
		r.Post("/send/{id}/retry", s.handlePeppolRetry)
		r.Get("/outbound", s.handlePeppolOutbound)
		r.Get("/messages", s.handlePeppolMessages)
		r.Get("/messages/{id}", s.handlePeppolMessageGet)
		r.Get("/participants", s.handlePeppolParticipantsList)
		r.Post("/participants", s.handlePeppolParticipantsAdd)
		r.Get("/status", s.handlePeppolStatus)
		r.Post("/ping", s.handlePeppolPing)
	})
}

// requirePeppolPlan gates the whole /peppol subtree for Solo / unlicensed boxes.
func (s *Server) requirePeppolPlan(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.getCaps().Peppol {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":  "plan_limit",
				"detail": "Peppol requires the Pro plan",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requirePeppol(w http.ResponseWriter) *peppol.Engine {
	if s.Peppol == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "peppol not configured"})
		return nil
	}
	return s.Peppol
}

func (s *Server) handlePeppolInboundPost(w http.ResponseWriter, r *http.Request) {
	eng := s.requirePeppol(w)
	if eng == nil {
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	res, err := eng.ReceiveSBD(data)
	if err != nil {
		if errors.Is(err, peppol.ErrBadEnvelope) {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
				"error":      "unrecognized peppol envelope",
				"detail":     err.Error(),
				"message_id": res.MessageID,
			})
			return
		}
		s.log.Printf("peppol inbound: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice_id":  res.InvoiceID,
		"status":      res.Status,
		"external_id": res.ExternalID,
		"message_id":  res.MessageID,
	})
}

func (s *Server) handlePeppolInboundGet(w http.ResponseWriter, r *http.Request) {
	eng := s.requirePeppol(w)
	if eng == nil {
		return
	}
	since := time.Time{}
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}
	results, err := eng.PollAndIngest(r.Context(), since)
	if err != nil {
		s.log.Printf("peppol poll: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ap unreachable"})
		return
	}
	if results == nil {
		results = []peppol.ReceiveResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"new": results})
}

func (s *Server) handlePeppolSend(w http.ResponseWriter, r *http.Request) {
	eng := s.requirePeppol(w)
	if eng == nil {
		return
	}
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var req peppol.SendRequest
	if err := json.Unmarshal(data, &req); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
		return
	}
	// Meter when building a new document (gobl body); existing invoice_id = 0.
	meter := len(req.GOBL) > 0 && req.InvoiceID == 0
	if meter && !s.checkKeyQuota(w, r) {
		return
	}
	res, err := eng.Send(r.Context(), req)
	if err == nil && meter {
		s.meterInvoiceOp(r)
	}
	s.writeSendResult(w, res, err)
}

func (s *Server) handlePeppolRetry(w http.ResponseWriter, r *http.Request) {
	eng := s.requirePeppol(w)
	if eng == nil {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	res, err := eng.Retry(r.Context(), id)
	s.writeSendResult(w, res, err)
}

func (s *Server) writeSendResult(w http.ResponseWriter, res peppol.SendResult, err error) {
	if err == nil {
		writeJSON(w, http.StatusCreated, map[string]any{
			"message_id": res.MessageID,
			"status":     res.Status,
			"invoice_id": res.InvoiceID,
			"receipt":    res.Receipt,
		})
		return
	}
	switch {
	case errors.Is(err, peppol.ErrValidation):
		body := map[string]any{"error": "validation failed"}
		if res.Report != nil {
			body["report"] = res.Report
		}
		writeJSON(w, http.StatusUnprocessableEntity, body)
	case errors.Is(err, peppol.ErrAPRejected):
		writeJSON(w, statusFailedDependency, map[string]any{
			"error":      "ap rejected",
			"detail":     res.Detail,
			"message_id": res.MessageID,
			"receipt":    res.Receipt,
		})
	case errors.Is(err, peppol.ErrAPUnreachable):
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":      "ap unreachable",
			"message_id": res.MessageID,
			"status":     res.Status,
		})
	case errors.Is(err, archive.ErrNotFound):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		s.log.Printf("peppol send: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func (s *Server) handlePeppolOutbound(w http.ResponseWriter, r *http.Request) {
	s.listPeppolMessages(w, r, archive.PeppolDirOut)
}

func (s *Server) handlePeppolMessages(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("direction")
	s.listPeppolMessages(w, r, dir)
}

func (s *Server) listPeppolMessages(w http.ResponseWriter, r *http.Request, dir string) {
	if s.Store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	status := r.URL.Query().Get("status")
	since := r.URL.Query().Get("since")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := s.Store.ListMessages(dir, status, since, limit)
	if err != nil {
		s.log.Printf("peppol list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, peppolMessageJSON(m, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

func (s *Server) handlePeppolMessageGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	m, err := s.Store.MessageByID(id)
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, peppolMessageJSON(m, true))
}

func peppolMessageJSON(m archive.PeppolMessage, withBlobs bool) map[string]any {
	out := map[string]any{
		"id":             m.ID,
		"direction":      m.Direction,
		"from_peppol_id": m.FromPeppolID,
		"to_peppol_id":   m.ToPeppolID,
		"ap_message_id":  m.APMessageID,
		"status":         m.Status,
		"invoice_id":     m.InvoiceID,
		"external_id":    m.ExternalID,
		"created_at":     m.CreatedAt,
		"updated_at":     m.UpdatedAt,
	}
	if withBlobs {
		out["bis_xml"] = string(m.BisXML)
		out["receipt"] = string(m.Receipt)
	}
	return out
}

func (s *Server) handlePeppolParticipantsList(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.Participants()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"participants": ps})
}

func (s *Server) handlePeppolParticipantsAdd(w http.ResponseWriter, r *http.Request) {
	data, err := readBodyLimited(w, r)
	if err != nil {
		return
	}
	var p archive.Participant
	if err := json.Unmarshal(data, &p); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(p.PeppolID) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "peppol_id required"})
		return
	}
	if err := s.Store.AddParticipant(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	// Optionally register with AP.
	if s.Peppol != nil && s.Peppol.AP != nil && p.IsSelf {
		ep, err := s.Peppol.AP.Register(r.Context(), peppol.Participant{
			ID: p.PeppolID, ServiceType: p.ServiceType, Name: p.Name, IsSelf: true,
		})
		if err == nil && ep != "" {
			p.RegisteredEndpoint = ep
			_ = s.Store.AddParticipant(p)
		}
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePeppolStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := peppol.ConfigFromEnv()
	apName := ""
	configured := false
	if s.Peppol != nil && s.Peppol.AP != nil {
		apName = s.Peppol.AP.Name()
		configured = true
	} else if cfg.AP != "" {
		apName = cfg.AP
	}
	selfID := cfg.SelfID
	if s.Store != nil {
		if p, err := s.Store.Self(); err == nil {
			selfID = p.PeppolID
		}
	}
	counts := map[string]int{}
	if s.Store != nil {
		counts, _ = s.Store.CountMessagesByStatus("")
		if counts == nil {
			counts = map[string]int{}
		}
	}
	poll := os.Getenv("FACTURA_PEPPOL_POLL") == "1"
	interval := os.Getenv("FACTURA_PEPPOL_POLL_INTERVAL")
	if interval == "" {
		interval = "60s"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"self_id":       selfID,
		"ap_name":       apName,
		"ap_configured": configured,
		"callback_url":  cfg.CallbackURL,
		"poll_enabled":  poll,
		"poll_interval": interval,
		"counts":        counts,
	})
}

func (s *Server) handlePeppolPing(w http.ResponseWriter, r *http.Request) {
	eng := s.requirePeppol(w)
	if eng == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := eng.AP.Ping(ctx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ap_name": eng.AP.Name()})
}
