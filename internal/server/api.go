package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

const maxJSONRequestBodyBytes = 4 << 20

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Best effort — headers already sent.
		_ = err
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON decodes the request body into v. Returns false and writes a 400
// error response if decoding fails.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any, logger *slog.Logger) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		logger.Warn("invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		logger.Warn("invalid request body", "error", "multiple JSON values")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
