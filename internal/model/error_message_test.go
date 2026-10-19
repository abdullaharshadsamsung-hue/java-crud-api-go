package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorMessageZeroValue(t *testing.T) {
	var e ErrorMessage
	if e.Status != "" || e.Message != "" {
		t.Fatalf("zero value not empty: %+v", e)
	}
}

func TestNewErrorMessage(t *testing.T) {
	tests := []struct {
		name, status, message string
	}{
		{"not found", StatusNotFound, "Contact not found"},
		{"empty both", "", ""},
		{"bad request", "BAD_REQUEST", "Invalid input"},
		{"no transform", "weird Status", "  spaced  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewErrorMessage(tt.status, tt.message)
			if e.Status != tt.status || e.Message != tt.message {
				t.Fatalf("got %+v", e)
			}
		})
	}
}

func TestErrorMessageSetFields(t *testing.T) {
	tests := []struct {
		name                  string
		setStatus, setMessage *string
		wantStatus, wantMsg   string
	}{
		{"set status", errMsgStrp("INTERNAL_SERVER_ERROR"), nil, "INTERNAL_SERVER_ERROR", "orig"},
		{"clear status", errMsgStrp(""), nil, "", "orig"},
		{"set message", nil, errMsgStrp("Invalid input"), "ORIG", "Invalid input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewErrorMessage("ORIG", "orig")
			if tt.setStatus != nil {
				e.Status = *tt.setStatus
			}
			if tt.setMessage != nil {
				e.Message = *tt.setMessage
			}
			if e.Status != tt.wantStatus || e.Message != tt.wantMsg {
				t.Fatalf("got %+v", e)
			}
		})
	}
}

func errMsgStrp(s string) *string { return &s }

func TestHTTPStatusName(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{404, "NOT_FOUND"},
		{500, "INTERNAL_SERVER_ERROR"},
		{400, "BAD_REQUEST"},
		{200, "OK"},
		{413, "PAYLOAD_TOO_LARGE"},
		{414, "URI_TOO_LONG"},
		{416, "REQUESTED_RANGE_NOT_SATISFIABLE"},
		{418, "I_AM_A_TEAPOT"},
		{203, "NON_AUTHORITATIVE_INFORMATION"},
		{999, "999"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := HTTPStatusName(tt.code); got != tt.want {
				t.Fatalf("HTTPStatusName(%d)=%q want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestNewErrorMessageFromCode(t *testing.T) {
	e := NewErrorMessageFromCode(http.StatusNotFound, "x")
	if !e.Equal(NewErrorMessage(StatusNotFound, "x")) {
		t.Fatalf("got %+v", e)
	}
}

func TestErrorMessageEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b ErrorMessage
		want bool
	}{
		{"same", NewErrorMessage("NOT_FOUND", "m"), NewErrorMessage("NOT_FOUND", "m"), true},
		{"both zero", ErrorMessage{}, ErrorMessage{}, true},
		{"diff status", NewErrorMessage("NOT_FOUND", "m"), NewErrorMessage("BAD_REQUEST", "m"), false},
		{"diff message", NewErrorMessage("NOT_FOUND", "m"), NewErrorMessage("NOT_FOUND", "n"), false},
		{"vs zero", NewErrorMessage("NOT_FOUND", "m"), ErrorMessage{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.a.Equal(tt.b) != tt.want || tt.b.Equal(tt.a) != tt.want {
				t.Fatalf("Equal mismatch, want %v", tt.want)
			}
			if !tt.a.Equal(tt.a) {
				t.Fatal("not reflexive")
			}
			if tt.want && tt.a != tt.b {
				t.Fatal("equal values must compare == (hash-key consistency)")
			}
		})
	}
	a, b, c := NewErrorMessage("S", "m"), NewErrorMessage("S", "m"), NewErrorMessage("S", "m")
	if !(a.Equal(b) && b.Equal(c) && a.Equal(c)) {
		t.Fatal("not transitive")
	}
	m := map[ErrorMessage]int{a: 1}
	if m[b] != 1 {
		t.Fatal("equal values should hash equally")
	}
}

func TestErrorMessageString(t *testing.T) {
	tests := []struct {
		name string
		e    ErrorMessage
		want string
	}{
		{"filled", NewErrorMessage(StatusNotFound, "x"), "ErrorMessage(status=NOT_FOUND, message=x)"},
		{"zero", ErrorMessage{}, "ErrorMessage(status=, message=)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.e.String()
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if !strings.Contains(got, tt.e.Status) || !strings.Contains(got, tt.e.Message) {
				t.Fatal("missing fields")
			}
		})
	}
}

func TestErrorMessageJSON(t *testing.T) {
	tests := []struct {
		name string
		e    ErrorMessage
		want string
	}{
		{"404", NewErrorMessage(StatusNotFound, "User are not available"), `{"status":"NOT_FOUND","message":"User are not available"}`},
		{"zero", ErrorMessage{}, `{"status":"","message":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.e)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.want {
				t.Fatalf("got %s want %s", b, tt.want)
			}
			var back ErrorMessage
			if err := json.Unmarshal(b, &back); err != nil || !back.Equal(tt.e) {
				t.Fatalf("roundtrip failed: %v %+v", err, back)
			}
		})
	}
}

func TestErrorMessageJSONOverHTTP(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(NewErrorMessageFromCode(http.StatusNotFound, "User are not available"))
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"NOT_FOUND","message":"User are not available"}` {
		t.Fatalf("body %s", got)
	}
}