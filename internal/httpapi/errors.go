// Package httpapi contains the HTTP transport layer of the application.
//
// This file replaces the Spring @ControllerAdvice
// com.smartContact.error.RestResponseEntityExceptionHandling. Go has no global
// exception interception, so every handler sends its error to WriteError,
// which picks the status code and body the Spring application would have
// produced.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
)

// ErrBadRequest marks client input errors such as validation failures,
// malformed JSON bodies or invalid path ids. Spring's
// ResponseEntityExceptionHandler (the superclass of the source advice)
// answers these with 400 and an empty body. WriteError does the same.
var ErrBadRequest = errors.New("bad request")

// WriteError converts err into an HTTP response. It is the Go equivalent of
// the @ControllerAdvice/@ExceptionHandler dispatch:
//
//   - a user-not-found error (apperror.ErrUserNotFound / *apperror.UserNotFoundError)
//     -> 404 with the ErrorMessage body {"status":"NOT_FOUND","message":<msg>}
//   - ErrBadRequest -> 400 with an empty body (inherited
//     ResponseEntityExceptionHandler behaviour)
//   - anything else -> 500 with Spring's BasicErrorController JSON body
//
// A nil err writes nothing.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	if msg, ok := apperror.Message(err); ok {
		WriteUserNotFound(w, msg)
		return
	}
	if errors.Is(err, ErrBadRequest) {
		WriteEmptyStatus(w, http.StatusBadRequest)
		return
	}
	slog.ErrorContext(r.Context(), "unhandled error", "method", r.Method, "path", r.URL.Path, "error", err)
	WriteSpringError(w, r, http.StatusInternalServerError)
}

// WriteUserNotFound writes 404 Not Found with an ErrorMessage JSON body
// carrying the NOT_FOUND status and msg. It mirrors the source's
// userNotFoundException handler method.
//
// MIGRATION_NOTE: Java would serialize a null exception message as
// "message":null. Here an empty message is rendered as "". This only changes
// the external API when a UserNotFoundError has no message, which the
// service layer never creates (it always uses UserNotAvailableMessage).
func WriteUserNotFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, model.NewErrorMessage(model.StatusNotFound, msg))
}

// WriteEmptyStatus writes status with no body. It reproduces how Spring's
// ResponseEntityExceptionHandler answers framework exceptions such as
// MethodArgumentNotValidException, HttpMessageNotReadableException and
// MethodArgumentTypeMismatchException.
func WriteEmptyStatus(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

// springErrorBody is the JSON shape Spring Boot's BasicErrorController
// produces for errors that no advice handled.
type springErrorBody struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Path      string `json:"path"`
}

// WriteSpringError writes Spring Boot's default error JSON
// {timestamp, status, error, path}. Use it for 500s and for 404s on unknown
// routes.
//
// MIGRATION_NOTE: Spring prints the timestamp with millisecond precision and
// an offset. Here it is RFC 3339 in UTC. Clients that parse it as ISO-8601
// are unaffected.
func WriteSpringError(w http.ResponseWriter, r *http.Request, status int) {
	writeJSON(w, status, springErrorBody{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    status,
		Error:     http.StatusText(status),
		Path:      r.URL.Path,
	})
}

// writeJSON encodes v as the JSON response body with the given status.
// Encoding errors are logged because the status line has already been sent.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "status", status, "error", err)
	}
}
