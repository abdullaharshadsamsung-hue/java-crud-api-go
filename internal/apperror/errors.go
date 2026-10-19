// Package apperror defines the application's domain errors.
//
// It replaces the Java exception hierarchy in com.smartContact.error with
// idiomatic Go error values: a sentinel for errors.Is checks and a concrete
// type for errors.As when the caller needs the detail message (e.g. the HTTP
// layer rendering the 404 ErrorMessage body).
package apperror

import "errors"

// UserNotAvailableMessage is the detail message the service layer uses when a
// user lookup by id finds nothing (UserServiceImp.fetchUserById).
const UserNotAvailableMessage = "User are not available"

// ErrUserNotFound is the sentinel for "requested user does not exist".
// Every *UserNotFoundError matches it via errors.Is, so callers can either
// return ErrUserNotFound directly, wrap it with fmt.Errorf("...: %w", ...),
// or return a *UserNotFoundError carrying a custom message.
// The HTTP layer maps it to 404 Not Found.
var ErrUserNotFound = errors.New(UserNotAvailableMessage)

// ErrNotFoundRow signals that a repository operation (e.g. delete by id)
// affected no rows. It is deliberately distinct from ErrUserNotFound and must
// NOT be translated into a 404 response: the source's delete path does not
// raise UserNotFoundException.
var ErrNotFoundRow = errors.New("no row found")

// UserNotFoundError is the Go equivalent of the Java checked exception
// UserNotFoundException. Msg is the detail message (may be empty, mirroring a
// null Java message) and Cause is the optional underlying error.
type UserNotFoundError struct {
	Msg   string
	Cause error
}

// NewUserNotFound returns a UserNotFoundError with no detail message and no
// cause (Java: new UserNotFoundException()).
func NewUserNotFound() *UserNotFoundError {
	return &UserNotFoundError{}
}

// NewUserNotFoundMessage returns a UserNotFoundError with the given detail
// message (Java: new UserNotFoundException(message)).
func NewUserNotFoundMessage(msg string) *UserNotFoundError {
	return &UserNotFoundError{Msg: msg}
}

// NewUserNotFoundWrap returns a UserNotFoundError with a detail message and an
// underlying cause (Java: new UserNotFoundException(message, cause)).
func NewUserNotFoundWrap(msg string, cause error) *UserNotFoundError {
	return &UserNotFoundError{Msg: msg, Cause: cause}
}

// NewUserNotFoundFromCause returns a UserNotFoundError wrapping cause, whose
// message is derived from the cause (Java: new UserNotFoundException(cause),
// where the message becomes cause.toString(), or null when cause is null).
func NewUserNotFoundFromCause(cause error) *UserNotFoundError {
	e := &UserNotFoundError{Cause: cause}
	if cause != nil {
		e.Msg = cause.Error()
	}
	return e
}

// MIGRATION_NOTE: the protected Java constructor
// UserNotFoundException(String, Throwable, boolean enableSuppression,
// boolean writableStackTrace) has no Go equivalent: Go errors carry no stack
// trace and have no suppressed-exception list. Use NewUserNotFoundWrap; the
// two boolean flags are intentionally dropped.

// Error returns the detail message. An empty message mirrors Java's null
// getMessage(); when a no-message error is rendered, the handler emits "".
func (e *UserNotFoundError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

// Unwrap returns the underlying cause, enabling errors.Is/errors.As chains.
func (e *UserNotFoundError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is reports whether target is ErrUserNotFound, so any *UserNotFoundError
// satisfies errors.Is(err, ErrUserNotFound).
func (e *UserNotFoundError) Is(target error) bool {
	return target == ErrUserNotFound
}

// Message extracts the user-facing detail message for a user-not-found error
// anywhere in err's chain. It returns ok=false if err is not a user-not-found
// error. For a bare ErrUserNotFound sentinel the message is
// UserNotAvailableMessage.
func Message(err error) (msg string, ok bool) {
	var unf *UserNotFoundError
	if errors.As(err, &unf) {
		return unf.Msg, true
	}
	if errors.Is(err, ErrUserNotFound) {
		return UserNotAvailableMessage, true
	}
	return "", false
}
