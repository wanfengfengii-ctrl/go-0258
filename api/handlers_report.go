package api

import (
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) {
	id := inspection.TaskID(r.PathValue("id"))
	report, fault := h.svc.BuildReport(r.Context(), id)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
