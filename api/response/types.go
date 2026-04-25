// Package response defines the JSON envelope types used by all HTTP handlers.
package response

import (
	"encoding/json"
	"net/http"
)

// Envelope wraps a successful API response.
type Envelope struct {
	Data any  `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

// Meta holds pagination metadata.
type Meta struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// ErrorEnvelope wraps an error response.
type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

// APIError is a structured error payload.
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON writes status and v as JSON. On marshal failure it falls back to 500.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// OK writes a 200 JSON response with the data envelope.
func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Envelope{Data: data})
}

// OKPaged writes a 200 JSON response with data and pagination meta.
func OKPaged(w http.ResponseWriter, data any, total int64, limit, offset int) {
	JSON(w, http.StatusOK, Envelope{
		Data: data,
		Meta: &Meta{Total: total, Limit: limit, Offset: offset},
	})
}
