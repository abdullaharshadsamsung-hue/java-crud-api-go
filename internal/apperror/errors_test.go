package apperror

import (
	"errors"
	"fmt"
	"testing"
)

type customErr struct{ s string }

func (c *customErr) Error() string { return c.s }

func TestNewUserNotFound(t *testing.T) {
	e := NewUserNotFound()
	if e == nil {
		t.Fatal("nil")
	}
	if e.Msg != "" || e.Error() != "" {
		t.Errorf("msg = %q, want empty", e.Msg)
	}
	if e.Cause != nil || e.Unwrap() != nil {
		t.Errorf("cause should be nil")
	}
	if !errors.Is(e, ErrUserNotFound) {
		t.Errorf("should match ErrUserNotFound")
	}
	// distinct instances
	if NewUserNotFound() == e {
		t.Errorf("expected new instance")
	}
}

func TestNewUserNotFoundMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"with message", "User not found"},
		{"empty (null) message", ""},
		{"service message", UserNotAvailableMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewUserNotFoundMessage(tt.msg)
			if e.Error() != tt.msg || e.Msg != tt.msg {
				t.Errorf("got %q want %q", e.Error(), tt.msg)
			}
			if e.Unwrap() != nil {
				t.Errorf("cause should be nil")
			}
			if !errors.Is(e, ErrUserNotFound) {
				t.Errorf("should match sentinel")
			}
		})
	}
}

func TestNewUserNotFoundWrap(t *testing.T) {
	cause := &customErr{"db down"}
	tests := []struct {
		name  string
		msg   string
		cause error
	}{
		{"message and cause", "missing", cause},
		{"nil cause", "missing", nil},
		{"empty message with cause", "", cause},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewUserNotFoundWrap(tt.msg, tt.cause)
			if e.Error() != tt.msg {
				t.Errorf("msg %q want %q (must not derive from cause)", e.Error(), tt.msg)
			}
			if e.Unwrap() != tt.cause {
				t.Errorf("cause not identical")
			}
			if tt.cause != nil {
				var ce *customErr
				if !errors.As(e, &ce) || ce != cause {
					t.Errorf("errors.As failed on cause chain")
				}
				if !errors.Is(e, cause) {
					t.Errorf("errors.Is cause failed")
				}
			}
		})
	}
}

func TestNewUserNotFoundFromCause(t *testing.T) {
	cause := errors.New("boom")
	tests := []struct {
		name    string
		cause   error
		wantMsg string
	}{
		{"non-nil cause", cause, "boom"},
		{"nil cause", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewUserNotFoundFromCause(tt.cause)
			if e.Error() != tt.wantMsg {
				t.Errorf("msg %q want %q", e.Error(), tt.wantMsg)
			}
			if e.Unwrap() != tt.cause {
				t.Errorf("cause mismatch")
			}
		})
	}
}

func TestNilReceiver(t *testing.T) {
	var e *UserNotFoundError
	if e.Error() != "" {
		t.Errorf("nil Error should be empty")
	}
	if e.Unwrap() != nil {
		t.Errorf("nil Unwrap should be nil")
	}
}

func TestIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"struct matches sentinel", NewUserNotFound(), ErrUserNotFound, true},
		{"wrapped struct matches", fmt.Errorf("x: %w", NewUserNotFoundMessage("m")), ErrUserNotFound, true},
		{"struct not NotFoundRow", NewUserNotFound(), ErrNotFoundRow, false},
		{"NotFoundRow not user", ErrNotFoundRow, ErrUserNotFound, false},
		{"sentinel itself", ErrUserNotFound, ErrUserNotFound, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
		wantOK  bool
	}{
		{"nil", nil, "", false},
		{"unrelated", errors.New("other"), "", false},
		{"not found row", ErrNotFoundRow, "", false},
		{"sentinel", ErrUserNotFound, UserNotAvailableMessage, true},
		{"wrapped sentinel", fmt.Errorf("svc: %w", ErrUserNotFound), UserNotAvailableMessage, true},
		{"struct with msg", NewUserNotFoundMessage("custom"), "custom", true},
		{"struct empty msg", NewUserNotFound(), "", true},
		{"wrapped struct", fmt.Errorf("a: %w", NewUserNotFoundWrap("deep", errors.New("c"))), "deep", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := Message(tt.err)
			if msg != tt.wantMsg || ok != tt.wantOK {
				t.Errorf("got (%q,%v) want (%q,%v)", msg, ok, tt.wantMsg, tt.wantOK)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if UserNotAvailableMessage != "User are not available" {
		t.Errorf("unexpected message %q", UserNotAvailableMessage)
	}
	if ErrUserNotFound.Error() != UserNotAvailableMessage {
		t.Errorf("sentinel message mismatch")
	}
	if ErrNotFoundRow.Error() != "no row found" {
		t.Errorf("unexpected row msg")
	}
	var _ error = (*UserNotFoundError)(nil)
}