package web

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors that handlers return; RenderError maps them to HTTP status codes.
var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation error")

	// ErrEmailTaken is returned when an email is already linked to another
	// account. It wraps ErrConflict so RenderError maps it to 409.
	ErrEmailTaken = fmt.Errorf("email already in use: %w", ErrConflict)
)

// RenderError writes an appropriate HTTP error response for err.
// Sentinel errors (and errors that wrap them) map to specific status codes;
// all other errors produce a 500 Internal Server Error.
func RenderError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, ErrConflict):
		code = http.StatusConflict
	case errors.Is(err, ErrValidation):
		code = http.StatusBadRequest
	}
	http.Error(w, http.StatusText(code), code)
}
