package service

import (
	"context"
	"errors"
	"sort"
	"testing"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

// fakeRepo is an in-memory repository.UserRepository mimicking JPA semantics.
type fakeRepo struct {
	users   map[int]model.User
	nextID  int
	err     error // forced error for all operations
	saves   int
	deletes int
	lastIn  *model.User
}

var _ repository.UserRepository = (*fakeRepo)(nil)

func newFakeRepo(users ...model.User) *fakeRepo {
	r := &fakeRepo{users: map[int]model.User{}, nextID: 1}
	for _, u := range users {
		r.users[u.ID] = u
		if u.ID >= r.nextID {
			r.nextID = u.ID + 1
		}
	}
	return r
}

func (r *fakeRepo) Save(_ context.Context, u *model.User) (*model.User, error) {
	r.saves++
	r.lastIn = u
	if r.err != nil {
		return nil, r.err
	}
	out := *u
	if _, ok := r.users[out.ID]; out.ID == 0 || !ok {
		out.ID = r.nextID
		r.nextID++
	}
	r.users[out.ID] = out
	return &out, nil
}

func (r *fakeRepo) FindAll(context.Context) ([]model.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]model.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) FindPage(context.Context, int, int) ([]model.User, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepo) FindByID(_ context.Context, id int) (*model.User, bool, error) {
	if r.err != nil {
		return nil, false, r.err
	}
	u, ok := r.users[id]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func (r *fakeRepo) FindByName(_ context.Context, name string) (*model.User, bool, error) {
	if r.err != nil {
		return nil, false, r.err
	}
	var found []model.User
	for _, u := range r.users {
		if u.Name != nil && *u.Name == name {
			found = append(found, u)
		}
	}
	switch len(found) {
	case 0:
		return nil, false, nil
	case 1:
		return &found[0], true, nil
	default:
		return nil, false, repository.ErrNonUniqueName
	}
}

func (r *fakeRepo) DeleteByID(_ context.Context, id int) error {
	r.deletes++
	if r.err != nil {
		return r.err
	}
	if _, ok := r.users[id]; !ok {
		return apperror.ErrNotFoundRow
	}
	delete(r.users, id)
	return nil
}

func sp(s string) *string { return &s }

func mustService(t *testing.T, r repository.UserRepository) *UserService {
	t.Helper()
	s, err := NewUserService(r)
	if err != nil {
		t.Fatalf("NewUserService: %v", err)
	}
	return s
}

var errDB = errors.New("db down")

func TestNewUserService(t *testing.T) {
	tests := []struct {
		name    string
		repo    repository.UserRepository
		wantErr bool
	}{
		{"nil repo", nil, true},
		{"valid repo", newFakeRepo(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewUserService(tt.repo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && s == nil {
				t.Fatal("nil service")
			}
			if tt.wantErr && s != nil {
				t.Fatal("expected nil service")
			}
		})
	}
}

func TestSaveUser(t *testing.T) {
	existing := *model.NewUserBuilder().ID(1).Name("alice").Email("a@x").Build()
	tests := []struct {
		name     string
		repo     *fakeRepo
		in       *model.User
		wantID   int
		wantName string
		wantErr  error
	}{
		{"new user gets generated id", newFakeRepo(), model.NewUserBuilder().Name("bob").Build(), 1, "bob", nil},
		{"existing id is updated", newFakeRepo(existing), model.NewUserBuilder().ID(1).Name("alice2").Build(), 1, "alice2", nil},
		{"repo error wrapped", func() *fakeRepo { r := newFakeRepo(); r.err = errDB; return r }(), model.NewUserBuilder().Name("x").Build(), 0, "", errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			ctx := context.Background()
			got, err := s.SaveUser(ctx, tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Fatal("expected nil user on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.ID != tt.wantID || got.Name == nil || *got.Name != tt.wantName {
				t.Fatalf("got %v", got)
			}
			// Invariant: retrievable via FetchUserByID and reflects stored state.
			fetched, err := s.FetchUserByID(ctx, got.ID)
			if err != nil {
				t.Fatalf("fetch after save: %v", err)
			}
			if !fetched.Equal(got) {
				t.Fatalf("fetched %v != saved %v", fetched, got)
			}
			if len(tt.repo.users) != 1 {
				t.Fatalf("store size = %d, want 1", len(tt.repo.users))
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	u1 := *model.NewUserBuilder().ID(1).Name("a").Build()
	u2 := *model.NewUserBuilder().ID(2).Name("b").Build()
	tests := []struct {
		name    string
		repo    *fakeRepo
		want    []model.User
		wantErr bool
	}{
		{"users exist", newFakeRepo(u1, u2), []model.User{u1, u2}, false},
		{"empty store", newFakeRepo(), []model.User{}, false},
		{"repo error", func() *fakeRepo { r := newFakeRepo(); r.err = errDB; return r }(), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			before := len(tt.repo.users)
			got, err := s.FetchUserList(context.Background())
			if tt.wantErr {
				if !errors.Is(err, errDB) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil {
				t.Fatal("slice must not be nil")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !got[i].Equal(&tt.want[i]) {
					t.Fatalf("got[%d] = %v, want %v", i, &got[i], &tt.want[i])
				}
			}
			if tt.repo.saves != 0 || tt.repo.deletes != 0 || len(tt.repo.users) != before {
				t.Fatal("read must not modify store")
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	u1 := *model.NewUserBuilder().ID(7).Name("a").Build()
	tests := []struct {
		name         string
		repo         *fakeRepo
		id           int
		wantNotFound bool
		wantErr      error
	}{
		{"existing", newFakeRepo(u1), 7, false, nil},
		{"missing", newFakeRepo(u1), 99, true, nil},
		{"repo error", func() *fakeRepo { r := newFakeRepo(u1); r.err = errDB; return r }(), 7, false, errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			got, err := s.FetchUserByID(context.Background(), tt.id)
			switch {
			case tt.wantNotFound:
				if got != nil {
					t.Fatal("expected nil user")
				}
				if !errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatalf("err = %v, want ErrUserNotFound", err)
				}
				var unf *apperror.UserNotFoundError
				if !errors.As(err, &unf) || unf.Msg != apperror.UserNotAvailableMessage {
					t.Fatalf("want UserNotFoundError with message, got %v", err)
				}
				if msg, ok := apperror.Message(err); !ok || msg != "User are not available" {
					t.Fatalf("Message = %q, %v", msg, ok)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) || got != nil {
					t.Fatalf("err = %v, got = %v", err, got)
				}
				if errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatal("db error must not be not-found")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if got == nil || !got.Equal(&u1) {
					t.Fatalf("got %v", got)
				}
			}
			if tt.repo.saves != 0 || tt.repo.deletes != 0 {
				t.Fatal("read must not modify store")
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	u1 := *model.NewUserBuilder().ID(3).Name("a").Build()
	tests := []struct {
		name    string
		repo    *fakeRepo
		id      int
		wantErr error
	}{
		{"existing", newFakeRepo(u1), 3, nil},
		{"missing", newFakeRepo(u1), 4, apperror.ErrNotFoundRow},
		{"repo error", func() *fakeRepo { r := newFakeRepo(u1); r.err = errDB; return r }(), 3, errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			ctx := context.Background()
			err := s.DeleteUser(ctx, tt.id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatal("delete error must not map to ErrUserNotFound")
				}
				if _, ok := apperror.Message(err); ok {
					t.Fatal("delete error must not be a user-not-found error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if _, err := s.FetchUserByID(ctx, tt.id); !errors.Is(err, apperror.ErrUserNotFound) {
				t.Fatalf("after delete, fetch err = %v", err)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	u1 := *model.NewUserBuilder().ID(5).Name("old").Email("o@x").Build()
	tests := []struct {
		name      string
		repo      *fakeRepo
		id        int
		in        *model.User
		wantErr   error
		anyErr    bool
		checkID   int
		wantName  string
		wantEmail *string
	}{
		{"existing id updated", newFakeRepo(u1), 5, model.NewUserBuilder().ID(999).Name("new").Build(), nil, false, 5, "new", nil},
		{"missing id inserts new", newFakeRepo(u1), 42, model.NewUserBuilder().Name("ins").Build(), nil, false, 6, "ins", nil},
		{"nil user", newFakeRepo(u1), 5, nil, nil, true, 0, "", nil},
		{"repo error", func() *fakeRepo { r := newFakeRepo(u1); r.err = errDB; return r }(), 5, model.NewUserBuilder().Name("x").Build(), errDB, true, 0, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			err := s.UpdateUser(context.Background(), tt.id, tt.in)
			if tt.anyErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if tt.in == nil && tt.repo.saves != 0 {
					t.Fatal("repo must not be called for nil user")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tt.in.ID != tt.id {
				t.Fatalf("argument ID = %d, want overwritten to %d", tt.in.ID, tt.id)
			}
			if tt.repo.lastIn == nil || tt.repo.lastIn.ID != tt.id {
				t.Fatal("repo Save must receive user with id parameter")
			}
			got, ok := tt.repo.users[tt.checkID]
			if !ok || got.Name == nil || *got.Name != tt.wantName {
				t.Fatalf("stored = %v", &got)
			}
			if got.Email != tt.wantEmail {
				t.Fatalf("nil fields must be written as nil, got email %v", got.Email)
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	a := *model.NewUser(1, sp("alice"), sp("a@x"), nil, nil, nil)
	d1 := *model.NewUser(2, sp("dup"), nil, nil, nil, nil)
	d2 := *model.NewUser(3, sp("dup"), nil, nil, nil, nil)
	tests := []struct {
		name    string
		repo    *fakeRepo
		query   string
		wantOK  bool
		wantErr error
	}{
		{"match", newFakeRepo(a), "alice", true, nil},
		{"no match", newFakeRepo(a), "bob", false, nil},
		{"non unique", newFakeRepo(a, d1, d2), "dup", false, repository.ErrNonUniqueName},
		{"repo error", func() *fakeRepo { r := newFakeRepo(a); r.err = errDB; return r }(), "alice", false, errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustService(t, tt.repo)
			u, ok, err := s.GetUserNameByName(context.Background(), tt.query)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || u != nil || ok {
					t.Fatalf("u=%v ok=%v err=%v", u, ok, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK {
				if u == nil || !u.Equal(&a) {
					t.Fatalf("got %v", u)
				}
			} else if u != nil {
				t.Fatalf("expected nil user, got %v", u)
			}
			if tt.repo.saves != 0 || tt.repo.deletes != 0 {
				t.Fatal("read must not modify store")
			}
		})
	}
}