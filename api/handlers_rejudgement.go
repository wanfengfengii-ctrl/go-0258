package api

import (
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
)

func (h *Handler) handleRejudge(w http.ResponseWriter, r *http.Request) {
	id := inspection.TaskID(r.PathValue("id"))
	var req service.RejudgementRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, fault := h.svc.Rejudge(r.Context(), id, req)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleReview(w http.ResponseWriter, r *http.Request) {
	id := inspection.TaskID(r.PathValue("id"))
	var req service.ReviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, fault := h.svc.Review(r.Context(), id, req)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
