// Package api exposes the JSON HTTP surface of the inspection console and
// serves the embedded browser console static assets. Handlers delegate to the
// service layer and translate stable faults into deterministic HTTP error
// responses with sorted reasons.
package api

import (
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
)

// Handler wires the JSON API to the service.
type Handler struct {
	svc *service.Service
	mux *http.ServeMux
}

// NewHandler builds the API router over a service.
func NewHandler(svc *service.Service) *Handler {
	h := &Handler{svc: svc, mux: http.NewServeMux()}
	h.routes()
	return h
}

// Service exposes the backing service (used by tests).
func (h *Handler) Service() *service.Service { return h.svc }

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /api/health", h.handleHealth)
	h.mux.HandleFunc("GET /api/tasks", h.handleListTasks)
	h.mux.HandleFunc("POST /api/tasks", h.handleCreateTask)
	h.mux.HandleFunc("GET /api/tasks/{id}", h.handleGetTask)
	h.mux.HandleFunc("POST /api/tasks/{id}/sampling-confirmations", h.handleSamplingConfirm)
	h.mux.HandleFunc("POST /api/tasks/{id}/blind-splits", h.handleBlindSplit)
	h.mux.HandleFunc("POST /api/tasks/{id}/occupancies", h.handleOccupancy)
	h.mux.HandleFunc("POST /api/tasks/{id}/cold-chain/readings", h.handleColdChain)
	h.mux.HandleFunc("POST /api/tasks/{id}/readings", h.handleReading)
	h.mux.HandleFunc("POST /api/tasks/{id}/rejudgements", h.handleRejudge)
	h.mux.HandleFunc("POST /api/tasks/{id}/reviews", h.handleReview)
	h.mux.HandleFunc("POST /api/tasks/{id}/finalize", h.handleFinalize)
	h.mux.HandleFunc("GET /api/tasks/{id}/report", h.handleReport)
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
