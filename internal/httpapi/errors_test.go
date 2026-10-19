package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWriteError_UserNotFound(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"custom message", apperror.NewUserNotFoundMessage("User not found with id 5"), "User not found with id 5"},
		{"nil message renders empty", apperror.NewUserNotFound(), ""},
		{"empty string message", apperror.NewUserNotFoundMessage(""), ""},
		{"sentinel", apperror.ErrUserNotFound, apperror.UserNotAvailableMessage},
		{"wrapped sentinel", fmt.Errorf("service: %w", apperror.ErrUserNotFound), apperror.UserNotAvailableMessage},
		{"wrapped typed (subclass analogue)", fmt.Errorf("ctx: %w", apperror.NewUserNotFoundMessage("gone")), "gone"},
		{"with cause", apperror.NewUserNotFoundWrap("wrapped msg", errors.New("db")), "wrapped msg"},
		{"from cause", apperror.NewUserNotFoundFromCause(errors.New("root")), "root"},
		{"service message", apperror.NewUserNotFoundMessage(apperror.UserNotAvailableMessage), apperror.UserNotAvailableMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/users/5", nil)
			WriteError(rec, req, tt.err)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode: %v; body=%q", err, rec.Body.String())
			}
			if len(raw) != 2 {
				t.Errorf("unexpected fields: %v", raw)
			}
			var em model.ErrorMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &em); err != nil {
				t.Fatal(err)
			}
			want := model.NewErrorMessage(model.StatusNotFound, tt.wantMsg)
			if !em.Equal(want) {
				t.Errorf("body = %v, want %v", em, want)
			}
		})
	}
}

func TestWriteEmptyStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteEmptyStatus(rec, status)
			if rec.Code != status {
				t.Errorf("status = %d, want %d", rec.Code, status)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q", rec.Body.String())
			}
		})
	}
}

func TestWriteSpringError(t *testing.T) {
	tests := []struct {
		status  int
		path    string
		wantErr string
	}{
		{http.StatusNotFound, "/unknown", "Not Found"},
		{http.StatusInternalServerError, "/users", "Internal Server Error"},
		{http.StatusMethodNotAllowed, "/users/1", "Method Not Allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.wantErr, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteSpringError(rec, httptest.NewRequest(http.MethodGet, tt.path, nil), tt.status)
			if rec.Code != tt.status {
				t.Fatalf("status = %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q", ct)
			}
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"timestamp", "status", "error", "path"} {
				if _, ok := raw[k]; !ok {
					t.Errorf("missing field %q", k)
				}
			}
			var body springErrorBody
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Status != tt.status || body.Error != tt.wantErr || body.Path != tt.path {
				t.Errorf("body = %+v", body)
			}
		})
	}
}

func TestWriteUserNotFound(t *testing.T) {
	for _, msg := range []string{"x", "", "User are not available"} {
		t.Run(msg, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteUserNotFound(rec, msg)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d", rec.Code)
			}
			var em model.ErrorMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &em); err != nil {
				t.Fatal(err)
			}
			if em.Status != model.StatusNotFound || em.Message != msg {
				t.Errorf("got %v", em)
			}
		})
	}
}