package api

import "net/http"

// httpStatusFor maps a stable fault code to an HTTP status code.
func httpStatusFor(code string) int {
	switch code {
	case "not_found":
		return http.StatusNotFound
	case "store_error":
		return http.StatusInternalServerError
	case "bad_request":
		return http.StatusBadRequest
	case "conflict", "content_conflict", "stale_generation", "terminal_state",
		"occupancy_conflict", "finalize_conflict", "rejudgement_exists",
		"duplicate_review":
		return http.StatusConflict
	default:
		return http.StatusUnprocessableEntity
	}
}
