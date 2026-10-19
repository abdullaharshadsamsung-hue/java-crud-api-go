package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
)

type fakeService struct {
	saveCalls   int
	savedUser   *model.User
	saveErr     error
	listCalls   int
	list        []model.User
	listErr     error
	byIDCalls   int
	byIDArg     int
	byIDUser    *model.User
	byIDErr     error
	delCalls    int
	delArg      int
	delErr      error
	updCalls    int
	updID       int
	updUser     *model.User
	updErr      error
	updMutate   bool
	nameCalls   int
	nameArg     string
	nameUser    *model.User
	nameOK      bool
	nameErr     error
	panicOnList bool
}

func (f *fakeService) SaveUser(ctx context.Context, u *model.User) (*model.User, error) {
	f.saveCalls++
	f.savedUser = u
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return u, nil
}

func (f *fakeService) FetchUserList(ctx context.Context) ([]model.User, error) {
	f.listCalls++
	if f.panicOnList {
		panic("boom")
	}
	return f.list, f.listErr
}

func (f *fakeService) FetchUserByID(ctx context.Context, id int) (*model.User, error) {
	f.byIDCalls++
	f.byIDArg = id
	return f.byIDUser, f.byIDErr
}

func (f *fakeService) DeleteUser(ctx context.Context, id int) error {
	f.delCalls++
	f.delArg = id
	return f.delErr
}

func (f *fakeService) UpdateUser(ctx context.Context, id int, u *model.User) error {
	f.updCalls++
	f.updID = id
	f.updUser = u
	if f.updMutate {
		u.ID = 999
		x := "mutated"
		u.Name = &x
	}
	return f.updErr
}

func (f *fakeService) GetUserNameByName(ctx context.Context, name string) (*model.User, bool, error) {
	f.nameCalls++
	f.nameArg = name
	return f.nameUser, f.nameOK, f.nameErr
}

func newTestHandler(t *testing.T, svc UserService) http.Handler {
	t.Helper()
	h, err := NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNewHandler(t *testing.T) {
	if _, err := NewHandler(nil, nil); err == nil {
		t.Fatal("expected error for nil service")
	}
	h, err := NewHandler(&fakeService{}, nil)
	if err != nil || h == nil {
		t.Fatalf("nil logger should be accepted: %v", err)
	}
}

func TestSaveUser(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		body      string
		saveErr   error
		wantCode  int
		wantBody  string
		wantCalls int
	}{
		{"valid", "/save_user_data", `{"name":"alice","email":"a@x","extra":1}`, nil, 200, msgUserSaved, 1},
		{"trailing slash", "/save_user_data/", `{"name":"alice"}`, nil, 200, msgUserSaved, 1},
		{"blank name", "/save_user_data", `{"name":"   "}`, nil, 400, "", 0},
		{"missing name", "/save_user_data", `{"email":"a@x"}`, nil, 400, "", 0},
		{"malformed", "/save_user_data", `{"name":`, nil, 400, "", 0},
		{"empty body", "/save_user_data", "", nil, 400, "", 0},
		{"null body", "/save_user_data", "null", nil, 400, "", 0},
		{"type mismatch", "/save_user_data", `{"name":5}`, nil, 400, "", 0},
		{"service error", "/save_user_data", `{"name":"alice"}`, errors.New("db"), 500, "", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{saveErr: tc.saveErr}
			rec := do(newTestHandler(t, svc), http.MethodPost, tc.path, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d want %d body=%q", rec.Code, tc.wantCode, rec.Body.String())
			}
			if svc.saveCalls != tc.wantCalls {
				t.Fatalf("calls=%d want %d", svc.saveCalls, tc.wantCalls)
			}
			switch tc.wantCode {
			case 200:
				if rec.Body.String() != tc.wantBody {
					t.Fatalf("body=%q", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); ct != textPlainUTF8 {
					t.Fatalf("content-type=%q", ct)
				}
				if svc.savedUser == nil || svc.savedUser.Name == nil || *svc.savedUser.Name != "alice" {
					t.Fatalf("saved user=%v", svc.savedUser)
				}
			case 400:
				if rec.Body.Len() != 0 {
					t.Fatalf("expected empty body, got %q", rec.Body.String())
				}
			case 500:
				var b map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b["status"].(float64) != 500 {
					t.Fatalf("bad 500 body %q", rec.Body.String())
				}
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	a, b := "a", "b"
	tests := []struct {
		name     string
		list     []model.User
		err      error
		wantCode int
		wantLen  int
	}{
		{"users", []model.User{{ID: 2, Name: &b}, {ID: 1, Name: &a}}, nil, 200, 2},
		{"nil list", nil, nil, 200, 0},
		{"empty list", []model.User{}, nil, 200, 0},
		{"error", nil, errors.New("db"), 500, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{list: tc.list, listErr: tc.err}
			rec := do(newTestHandler(t, svc), http.MethodGet, "/get_user_data", "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d", rec.Code)
			}
			if svc.listCalls != 1 {
				t.Fatalf("calls=%d", svc.listCalls)
			}
			if tc.wantCode != 200 {
				return
			}
			if strings.TrimSpace(rec.Body.String())[0] != '[' {
				t.Fatalf("not array: %q", rec.Body.String())
			}
			var got []model.User
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len=%d", len(got))
			}
			for i := range got {
				if !got[i].Equal(&tc.list[i]) {
					t.Fatalf("order/content mismatch at %d", i)
				}
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	n := "alice"
	tests := []struct {
		name      string
		path      string
		user      *model.User
		err       error
		wantCode  int
		wantCalls int
		wantMsg   string
	}{
		{"found", "/get_user_data/7", &model.User{ID: 7, Name: &n}, nil, 200, 1, ""},
		{"found slash", "/get_user_data/7/", &model.User{ID: 7, Name: &n}, nil, 200, 1, ""},
		{"not found", "/get_user_data/7", nil, apperror.NewUserNotFoundMessage(apperror.UserNotAvailableMessage), 404, 1, apperror.UserNotAvailableMessage},
		{"not found sentinel", "/get_user_data/7", nil, apperror.ErrUserNotFound, 404, 1, apperror.UserNotAvailableMessage},
		{"non int", "/get_user_data/abc", nil, nil, 400, 0, ""},
		{"overflow", "/get_user_data/99999999999", nil, nil, 400, 0, ""},
		{"service error", "/get_user_data/7", nil, errors.New("db"), 500, 1, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{byIDUser: tc.user, byIDErr: tc.err}
			rec := do(newTestHandler(t, svc), http.MethodGet, tc.path, "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
			}
			if svc.byIDCalls != tc.wantCalls {
				t.Fatalf("calls=%d", svc.byIDCalls)
			}
			switch tc.wantCode {
			case 200:
				if svc.byIDArg != 7 {
					t.Fatalf("arg=%d", svc.byIDArg)
				}
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Equal(tc.user) {
					t.Fatalf("got %v", got)
				}
			case 404:
				var b map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
					t.Fatal(err)
				}
				if b["message"] != tc.wantMsg {
					t.Fatalf("body=%v", b)
				}
			case 400:
				if rec.Body.Len() != 0 {
					t.Fatalf("expected empty body")
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		err       error
		wantCode  int
		wantCalls int
	}{
		{"ok", "/delete_user_data/3", nil, 200, 1},
		{"ok slash", "/delete_user_data/3/", nil, 200, 1},
		{"missing row", "/delete_user_data/3", apperror.ErrNotFoundRow, 500, 1},
		{"non int", "/delete_user_data/x", nil, 400, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{delErr: tc.err}
			rec := do(newTestHandler(t, svc), http.MethodDelete, tc.path, "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d", rec.Code)
			}
			if svc.delCalls != tc.wantCalls {
				t.Fatalf("calls=%d", svc.delCalls)
			}
			if tc.wantCode == 200 {
				if rec.Body.String() != "user data deleted Successfully" || svc.delArg != 3 {
					t.Fatalf("body=%q arg=%d", rec.Body.String(), svc.delArg)
				}
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		body      string
		err       error
		mutate    bool
		wantCode  int
		wantCalls int
		wantName  *string
	}{
		{"ok", "/update_user_data/5", `{"id":1,"name":"bob","email":"b@x"}`, nil, false, 200, 1, strPtr("bob")},
		{"no validation", "/update_user_data/5", `{"email":"b@x"}`, nil, false, 200, 1, nil},
		{"echo not mutated", "/update_user_data/5/", `{"name":"bob"}`, nil, true, 200, 1, strPtr("bob")},
		{"malformed", "/update_user_data/5", `{bad`, nil, false, 400, 0, nil},
		{"null body", "/update_user_data/5", `null`, nil, false, 400, 0, nil},
		{"non int", "/update_user_data/z", `{"name":"bob"}`, nil, false, 400, 0, nil},
		{"service error", "/update_user_data/5", `{"name":"bob"}`, errors.New("db"), false, 500, 1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{updErr: tc.err, updMutate: tc.mutate}
			rec := do(newTestHandler(t, svc), http.MethodPut, tc.path, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
			}
			if svc.updCalls != tc.wantCalls {
				t.Fatalf("calls=%d", svc.updCalls)
			}
			if tc.wantCode != 200 {
				return
			}
			if svc.updID != 5 {
				t.Fatalf("id=%d", svc.updID)
			}
			var got model.User
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.ID != 5 {
				t.Fatalf("echo id=%d", got.ID)
			}
			if (got.Name == nil) != (tc.wantName == nil) || (got.Name != nil && *got.Name != *tc.wantName) {
				t.Fatalf("echo name=%v", got.Name)
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	n := "carol"
	tests := []struct {
		name      string
		path      string
		user      *model.User
		ok        bool
		err       error
		wantCode  int
		wantEmpty bool
	}{
		{"found", "/get_user_name/name/carol", &model.User{ID: 1, Name: &n}, true, nil, 200, false},
		{"found slash", "/get_user_name/name/carol/", &model.User{ID: 1, Name: &n}, true, nil, 200, false},
		{"not found", "/get_user_name/name/carol", nil, false, nil, 200, true},
		{"ok but nil", "/get_user_name/name/carol", nil, true, nil, 200, true},
		{"error", "/get_user_name/name/carol", nil, false, errors.New("non unique"), 500, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{nameUser: tc.user, nameOK: tc.ok, nameErr: tc.err}
			rec := do(newTestHandler(t, svc), http.MethodGet, tc.path, "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d", rec.Code)
			}
			if svc.nameCalls != 1 || svc.nameArg != "carol" {
				t.Fatalf("calls=%d arg=%q", svc.nameCalls, svc.nameArg)
			}
			if tc.wantCode != 200 {
				return
			}
			if tc.wantEmpty {
				if rec.Body.Len() != 0 {
					t.Fatalf("expected empty body: %q", rec.Body.String())
				}
				return
			}
			var got model.User
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Equal(tc.user) {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestUnknownRoutesAndPanic(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		panic    bool
		wantCode int
	}{
		{"unknown path", http.MethodGet, "/nope", false, 404},
		{"wrong method", http.MethodGet, "/save_user_data", false, 404},
		{"panic recovered", http.MethodGet, "/get_user_data", true, 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{panicOnList: tc.panic}
			rec := do(newTestHandler(t, svc), tc.method, tc.path, "")
			if rec.Code != tc.wantCode {
				t.Fatalf("code=%d", rec.Code)
			}
			var b map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if b["path"] != tc.path || int(b["status"].(float64)) != tc.wantCode {
				t.Fatalf("body=%v", b)
			}
		})
	}
}

func strPtr(s string) *string { return &s }