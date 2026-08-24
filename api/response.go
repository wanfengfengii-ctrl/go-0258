package api

import (
	"encoding/json"
	"net/http"

	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
)

// writeJSON serializes a JSON response with a UTF-8 content type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError serializes a stable error response.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

// writeFault translates a service Fault into an HTTP error response with the
// stable code and sorted reasons.
func writeFault(w http.ResponseWriter, f *service.Fault) {
	status := httpStatusFor(f.Code)
	body := map[string]any{"error": f.Code}
	if len(f.Reasons) > 0 {
		body["reasons"] = f.Reasons
	}
	writeJSON(w, status, body)
}

// decodeJSON decodes a request body, returning false after writing a
// bad-request error on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return false
	}
	return true
}
