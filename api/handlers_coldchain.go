package api

import (
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
)

func (h *Handler) handleColdChain(w http.ResponseWriter, r *http.Request) {
	id := inspection.TaskID(r.PathValue("id"))
	var req service.ColdChainReadingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, fault := h.svc.ColdChainReadings(r.Context(), id, req)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
