package model

import (
	"fmt"
	"net/http"
	"strings"
)

// StatusNotFound is the JSON name Spring's HttpStatus.NOT_FOUND serializes to.
// It is the status the global exception handler uses for a missing user.
const StatusNotFound = "NOT_FOUND"

// ErrorMessage is the JSON error body returned to clients on failure.
// It replaces the Lombok @Data DTO com.smartContact.model.ErrorMessage.
//
// Status holds the Spring HttpStatus enum constant name (e.g. "NOT_FOUND")
// because that is how Jackson serializes the enum. The 404 body is exactly
// {"status":"NOT_FOUND","message":"User are not available"}.
//
// The zero value matches the source's no-args constructor: both fields unset.
// Status and Message are exported, so they replace the Lombok getters and setters.
type ErrorMessage struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewErrorMessage builds an ErrorMessage from a status name and a message, in
// declaration order. It mirrors the source's @AllArgsConstructor.
func NewErrorMessage(status, message string) ErrorMessage {
	return ErrorMessage{Status: status, Message: message}
}

// NewErrorMessageFromCode builds an ErrorMessage from a numeric HTTP status
// code. It converts the code to the Spring HttpStatus enum name with
// HTTPStatusName.
func NewErrorMessageFromCode(code int, message string) ErrorMessage {
	return ErrorMessage{Status: HTTPStatusName(code), Message: message}
}

// HTTPStatusName returns the Spring HttpStatus enum constant name for code
// (e.g. 404 -> "NOT_FOUND", 500 -> "INTERNAL_SERVER_ERROR"). It is derived
// from net/http's reason phrase, upper-cased, with spaces and hyphens turned
// into underscores. For an unknown code it returns the decimal code as a string.
//
// MIGRATION_NOTE: a few Spring enum names differ from the IANA reason
// phrases Go uses (e.g. 413 is PAYLOAD_TOO_LARGE in Spring but
// "Request Entity Too Large" in Go). Those cases are overridden below.
// Review this list if more statuses are used.
func HTTPStatusName(code int) string {
	switch code {
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusRequestURITooLong:
		return "URI_TOO_LONG"
	case http.StatusRequestedRangeNotSatisfiable:
		return "REQUESTED_RANGE_NOT_SATISFIABLE"
	case http.StatusTeapot:
		return "I_AM_A_TEAPOT"
	}
	text := http.StatusText(code)
	if text == "" {
		return fmt.Sprintf("%d", code)
	}
	r := strings.NewReplacer(" ", "_", "-", "_", "'", "")
	return strings.ToUpper(r.Replace(text))
}

// Equal reports whether e and other have the same status and message.
// It mirrors the value equality Lombok's @Data generates.
func (e ErrorMessage) Equal(other ErrorMessage) bool {
	return e.Status == other.Status && e.Message == other.Message
}

// String returns a representation that includes both fields, in the same
// format as Lombok's toString.
func (e ErrorMessage) String() string {
	return fmt.Sprintf("ErrorMessage(status=%s, message=%s)", e.Status, e.Message)
}
