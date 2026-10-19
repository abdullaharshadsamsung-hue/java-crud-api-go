package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"migrated-app/internal/model"
)

// UserService is the set of business operations the HTTP layer needs. It is
// declared on the consumer side, following Go convention. *service.UserService
// satisfies it.
type UserService interface {
	SaveUser(ctx context.Context, user *model.User) (*model.User, error)
	FetchUserList(ctx context.Context) ([]model.User, error)
	FetchUserByID(ctx context.Context, id int) (*model.User, error)
	DeleteUser(ctx context.Context, id int) error
	UpdateUser(ctx context.Context, id int, user *model.User) error
	GetUserNameByName(ctx context.Context, name string) (*model.User, bool, error)
}

const (
	textPlainUTF8 = "text/plain;charset=UTF-8"

	msgUserSaved   = "User data saved successfully!"
	msgUserDeleted = "user data deleted Successfully"
)

// userHandler replaces com.smartContact.Controller.UserController.
type userHandler struct {
	svc UserService
	log *slog.Logger
}

// NewHandler builds the application's HTTP handler. It registers every route
// of the source UserController on a Go 1.22 ServeMux and wraps the mux in
// panic-recovery middleware. If log is nil, slog.Default() is used.
func NewHandler(svc UserService, log *slog.Logger) (http.Handler, error) {
	if svc == nil {
		return nil, errors.New("httpapi: new handler: nil user service")
	}
	if log == nil {
		log = slog.Default()
	}
	h := &userHandler{svc: svc, log: log}

	mux := http.NewServeMux()
	// Spring MVC 5 matches trailing slashes by default, so each route is
	// also registered with a trailing slash using the {$} anchor.
	routes := []struct {
		pattern string
		fn      http.HandlerFunc
	}{
		{"POST /save_user_data", h.saveUser},
		{"GET /get_user_data", h.fetchUserList},
		{"GET /get_user_data/{id}", h.fetchUserByID},
		{"DELETE /delete_user_data/{id}", h.deleteUser},
		{"PUT /update_user_data/{id}", h.updateUser},
		{"GET /get_user_name/name/{name}", h.getUserNameByName},
	}
	for _, rt := range routes {
		mux.HandleFunc(rt.pattern, rt.fn)
		mux.HandleFunc(rt.pattern+"/{$}", rt.fn)
	}
	// Unknown paths get Spring Boot's default 404 JSON body.
	// MIGRATION_NOTE: because this catch-all matches every method, a known
	// path with the wrong method answers 404 here, where Spring answered 405.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteSpringError(w, r, http.StatusNotFound)
	})

	return recoverMiddleware(log, mux), nil
}

// saveUser handles POST /save_user_data (@Valid @RequestBody User).
func (h *userHandler) saveUser(w http.ResponseWriter, r *http.Request) {
	h.log.InfoContext(r.Context(), "inside the saveUser of UserController ")

	user, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := user.Validate(); err != nil {
		WriteError(w, r, fmt.Errorf("%w: %w", ErrBadRequest, err))
		return
	}
	if _, err := h.svc.SaveUser(r.Context(), user); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, msgUserSaved)
}

// fetchUserList handles GET /get_user_data.
func (h *userHandler) fetchUserList(w http.ResponseWriter, r *http.Request) {
	h.log.InfoContext(r.Context(), "inside the fetchUserList of UserController ")

	users, err := h.svc.FetchUserList(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if users == nil {
		users = []model.User{} // serialize as [] rather than null
	}
	writeJSON(w, http.StatusOK, users)
}

// fetchUserByID handles GET /get_user_data/{id}. A missing user maps to 404
// with the ErrorMessage body through WriteError.
func (h *userHandler) fetchUserByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	user, err := h.svc.FetchUserByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// deleteUser handles DELETE /delete_user_data/{id}. Deleting a missing id is
// a 500, as with Spring Data's EmptyResultDataAccessException.
func (h *userHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := h.svc.DeleteUser(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, msgUserDeleted)
}

// updateUser handles PUT /update_user_data/{id}. Validation is deliberately
// not applied, because the source has no @Valid here. The request body is
// echoed back with id set to the path id, as the source returned its mutated
// argument.
func (h *userHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	user, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	// Echo a copy so that changes the persistence layer makes to user (for
	// example a newly allocated id on insert) do not leak into the response.
	// Java echoed the detached argument, not the merged entity.
	echo := *user
	echo.ID = id

	if err := h.svc.UpdateUser(r.Context(), id, user); err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, &echo)
}

// getUserNameByName handles GET /get_user_name/name/{name}. When no user
// matches, the source returned null, which Spring renders as 200 with an
// empty body. Several matches give 500.
func (h *userHandler) getUserNameByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	user, ok, err := h.svc.GetUserNameByName(r.Context(), name)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if !ok || user == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// pathID parses the {id} path segment as a 32-bit signed integer, as Spring
// converts @PathVariable int. A failure wraps ErrBadRequest (400, empty body).
func pathID(r *http.Request) (int, error) {
	raw := r.PathValue("id")
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid id %q: %w", ErrBadRequest, raw, err)
	}
	return int(n), nil
}

// decodeUser reads a User from the JSON request body. Unknown properties are
// ignored (Jackson's Spring Boot default). An empty body, malformed JSON, a
// type mismatch or a literal null body all wrap ErrBadRequest, matching
// HttpMessageNotReadableException.
func decodeUser(r *http.Request) (*model.User, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("%w: missing request body", ErrBadRequest)
	}
	var user *model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("%w: decode user: %w", ErrBadRequest, err)
	}
	if user == nil {
		return nil, fmt.Errorf("%w: request body is null", ErrBadRequest)
	}
	return user, nil
}

// writeText writes a plain-text body, as Spring's StringHttpMessageConverter
// does for ResponseEntity<String>.
func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", textPlainUTF8)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		slog.Error("failed to write text response", "status", status, "error", err)
	}
}

// recoverMiddleware turns a panic in a handler into Spring's default 500 JSON
// instead of dropping the connection.
func recoverMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "panic serving request",
					"method", r.Method, "path", r.URL.Path, "panic", rec)
				WriteSpringError(w, r, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
