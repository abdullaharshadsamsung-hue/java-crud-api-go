package service

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

type impFakeRepo struct {
	users     map[int]model.User
	nextID    int
	saveErr   error
	findErr   error
	nameErr   error
	saveCalls []model.User
	writes    int
}

var _ repository.UserRepository = (*impFakeRepo)(nil)

func newImpFakeRepo() *impFakeRepo {
	return &impFakeRepo{users: map[int]model.User{}, nextID: 1}
}

func (f *impFakeRepo) Save(_ context.Context, u *model.User) (*model.User, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.writes++
	f.saveCalls = append(f.saveCalls, *u)
	out := *u
	if _, ok := f.users[out.ID]; !ok || out.ID == 0 {
		out.ID = f.nextID
		f.nextID++
	}
	f.users[out.ID] = out
	return &out, nil
}

func (f *impFakeRepo) FindAll(context.Context) ([]model.User, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	res := make([]model.User, 0, len(f.users))
	for _, u := range f.users {
		res = append(res, u)
	}
	return res, nil
}

func (f *impFakeRepo) FindPage(context.Context, int, int) ([]model.User, error) { return nil, nil }

func (f *impFakeRepo) FindByID(_ context.Context, id int) (*model.User, bool, error) {
	if f.findErr != nil {
		return nil, false, f.findErr
	}
	u, ok := f.users[id]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func (f *impFakeRepo) FindByName(_ context.Context, name string) (*model.User, bool, error) {
	if f.nameErr != nil {
		return nil, false, f.nameErr
	}
	for _, u := range f.users {
		if u.Name != nil && *u.Name == name {
			c := u
			return &c, true, nil
		}
	}
	return nil, false, nil
}

func (f *impFakeRepo) DeleteByID(_ context.Context, id int) error {
	if _, ok := f.users[id]; !ok {
		return apperror.ErrNotFoundRow
	}
	f.writes++
	delete(f.users, id)
	return nil
}

func newImpSvc(t *testing.T, r *impFakeRepo) UserOperations {
	t.Helper()
	s, err := NewUserService(r)
	if err != nil {
		t.Fatalf("NewUserService: %v", err)
	}
	return s
}

func impHemraj() model.User {
	return *model.NewUserBuilder().ID(3).Name("hemraj").Email("hemrajmalhi1234@gmail.com").
		About("Sr").Password("root").Role("java developer").Build()
}

func TestImpNewUserServiceNilRepo(t *testing.T) {
	if _, err := NewUserService(nil); err == nil {
		t.Fatal("expected error for nil repo")
	}
}

func TestImpGetUserNameByName(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		nameErr error
		wantOK  bool
		wantErr error
	}{
		{"found", "hemraj", nil, true, nil},
		{"not found", "nobody", nil, false, nil},
		{"non unique", "hemraj", repository.ErrNonUniqueName, false, repository.ErrNonUniqueName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			r.users[3] = impHemraj()
			r.nameErr = tt.nameErr
			s := newImpSvc(t, r)
			u, ok, err := s.GetUserNameByName(context.Background(), tt.query)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok {
				if u == nil || u.Name == nil || *u.Name != tt.query {
					t.Fatalf("got %v, want name %q", u, tt.query)
				}
			} else if u != nil {
				t.Fatalf("expected nil user, got %v", u)
			}
			if r.writes != 0 {
				t.Fatal("lookup must not modify data")
			}
		})
	}
}

func TestImpSaveUser(t *testing.T) {
	boom := errors.New("duplicate email")
	tests := []struct {
		name    string
		saveErr error
		wantID  int
	}{
		{"new user gets id", nil, 1},
		{"repository error propagates", boom, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			r.saveErr = tt.saveErr
			s := newImpSvc(t, r)
			in := model.NewUserBuilder().Name("a").Email("a@x").Build()
			got, err := s.SaveUser(context.Background(), in)
			if tt.saveErr != nil {
				if !errors.Is(err, tt.saveErr) || got != nil {
					t.Fatalf("got (%v,%v), want wrapped %v", got, err, tt.saveErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID || *got.Name != "a" {
				t.Fatalf("unexpected saved user %v", got)
			}
			stored := r.users[got.ID]
			if !got.Equal(&stored) {
				t.Fatalf("return %v differs from repo result %v", got, &stored)
			}
		})
	}
}

func TestImpFetchUserList(t *testing.T) {
	tests := []struct {
		name    string
		seed    []model.User
		findErr error
		want    int
	}{
		{"users exist", []model.User{impHemraj(), {ID: 4}}, nil, 2},
		{"empty", nil, nil, 0},
		{"error", nil, errors.New("db down"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			for _, u := range tt.seed {
				r.users[u.ID] = u
			}
			r.findErr = tt.findErr
			s := newImpSvc(t, r)
			got, err := s.FetchUserList(context.Background())
			if tt.findErr != nil {
				if !errors.Is(err, tt.findErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("list must not be nil")
			}
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			if r.writes != 0 {
				t.Fatal("must not modify data")
			}
		})
	}
}

func TestImpFetchUserByID(t *testing.T) {
	dbErr := errors.New("db down")
	tests := []struct {
		name     string
		id       int
		findErr  error
		notFound bool
	}{
		{"existing", 3, nil, false},
		{"missing", 99, nil, true},
		{"repo error", 3, dbErr, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			r.users[3] = impHemraj()
			r.findErr = tt.findErr
			s := newImpSvc(t, r)
			u, err := s.FetchUserByID(context.Background(), tt.id)
			switch {
			case tt.findErr != nil:
				if !errors.Is(err, dbErr) || errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatalf("err = %v", err)
				}
			case tt.notFound:
				if u != nil || !errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatalf("got (%v,%v)", u, err)
				}
				var unf *apperror.UserNotFoundError
				if !errors.As(err, &unf) || unf.Msg != "User are not available" {
					t.Fatalf("bad error %v", err)
				}
				if msg, ok := apperror.Message(err); !ok || msg != apperror.UserNotAvailableMessage {
					t.Fatalf("Message = %q,%v", msg, ok)
				}
			default:
				if err != nil || u == nil || u.ID != 3 {
					t.Fatalf("got (%v,%v)", u, err)
				}
			}
		})
	}
}

func TestImpDeleteUser(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		wantErr bool
	}{
		{"existing", 3, false},
		{"missing", 42, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			r.users[3] = impHemraj()
			s := newImpSvc(t, r)
			err := s.DeleteUser(context.Background(), tt.id)
			if tt.wantErr {
				if !errors.Is(err, apperror.ErrNotFoundRow) {
					t.Fatalf("err = %v", err)
				}
				if errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatal("delete must not produce user-not-found")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.FetchUserByID(context.Background(), tt.id); !errors.Is(err, apperror.ErrUserNotFound) {
				t.Fatalf("after delete err = %v", err)
			}
		})
	}
}

func TestImpUpdateUser(t *testing.T) {
	saveErr := errors.New("constraint")
	tests := []struct {
		name    string
		id      int
		user    *model.User
		saveErr error
		wantErr bool
	}{
		{"existing id overrides user id, nil fields overwrite", 3, &model.User{ID: 77, Name: impStrp("new")}, nil, false},
		{"non-existent id inserts", 50, &model.User{Name: impStrp("x")}, nil, false},
		{"nil user", 3, nil, nil, true},
		{"save error", 3, &model.User{Name: impStrp("x")}, saveErr, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newImpFakeRepo()
			r.users[3] = impHemraj()
			r.nextID = 10
			r.saveErr = tt.saveErr
			s := newImpSvc(t, r)
			err := s.UpdateUser(context.Background(), tt.id, tt.user)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.saveErr != nil && !errors.Is(err, tt.saveErr) {
					t.Fatalf("err = %v", err)
				}
				if errors.Is(err, apperror.ErrUserNotFound) {
					t.Fatal("must not be not-found")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.user.ID != tt.id {
				t.Fatalf("user.ID = %d, want %d", tt.user.ID, tt.id)
			}
			if len(r.saveCalls) != 1 || r.saveCalls[0].ID != tt.id {
				t.Fatalf("save calls %v", r.saveCalls)
			}
			if tt.id == 3 {
				got := r.users[3]
				if *got.Name != "new" || got.Email != nil || got.About != nil {
					t.Fatalf("record not replaced: %v", &got)
				}
			} else if _, ok := r.users[10]; !ok {
				t.Fatal("expected insert with generated id")
			}
		})
	}
}

func impStrp(s string) *string { return &s }