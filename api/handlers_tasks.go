package api

import (
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
)

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, fault := h.svc.ListTasks(r.Context())
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (h *Handler) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req service.CreateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, fault := h.svc.CreateTask(r.Context(), req)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := inspection.TaskID(r.PathValue("id"))
	snap, fault := h.svc.GetSnapshot(r.Context(), id)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
