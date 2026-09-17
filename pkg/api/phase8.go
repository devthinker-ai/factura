package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/devthinker-ai/factura/pkg/archive"
)

// GET /invoices/{id}/evidence — GoBD chain link for one invoice (viewer).
func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	ev, err := s.Store.EvidenceFor(id)
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		s.log.Printf("evidence: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice_id": ev.Link.InvoiceID,
		"hash":       ev.Link.Hash,
		"prev_hash":  ev.Link.PrevHash,
		"created_at": ev.Link.CreatedAt,
		"verified":   ev.OK,
	})
}

// GET /audit — full GoBD chain verification (viewer).
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	rep, err := s.Store.VerifyChain()
	if err != nil {
		s.log.Printf("audit: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              rep.OK,
		"link_count":      rep.LinkCount,
		"head":            rep.Head,
		"first_broken_id": rep.FirstBrokenID,
	})
}
