package response

import (
	"errors"
	"net/http"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog/store"
)

// Err writes a structured error response.
func Err(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorEnvelope{Error: APIError{Code: code, Message: message}})
}

// NotFound writes a 404 response.
func NotFound(w http.ResponseWriter, message string) {
	Err(w, http.StatusNotFound, "NOT_FOUND", message)
}

// BadRequest writes a 400 response.
func BadRequest(w http.ResponseWriter, message string) {
	Err(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

// InternalError writes a 500 response.
func InternalError(w http.ResponseWriter, message string) {
	Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", message)
}

// HandleStoreError maps common store errors to appropriate HTTP responses.
func HandleStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		NotFound(w, err.Error())
		return
	}
	InternalError(w, err.Error())
}
