// Package httpx holds small HTTP helpers shared across feature packages so each
// handler writes JSON and errors in the same consistent shape.
package httpx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// errorResponse is the consistent error envelope: {"error": "message"}.
type errorResponse struct {
	Error string `json:"error"`
}

// WriteJSON serializes payload as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes {"error": message} with the given status code.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, errorResponse{Error: message})
}

// DecodeJSON strictly decodes the request body into dst, rejecting unknown fields.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain a single JSON value")
		}
		return fmt.Errorf("request body must contain a single JSON value: %w", err)
	}
	return nil
}
