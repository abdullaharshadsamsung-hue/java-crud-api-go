package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sort"
	"sync"
	"testing"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
)

// ---------------------------------------------------------------------------
// In-memory fake database/sql driver (stdlib only) acting as the DB mock.
// ---------------------------------------------------------------------------

type fakeRow struct {
	id                                 int64
	name, email, password, role, about driver.Value
}

type fakeStore struct {
	mu       sync.Mutex
	users    map[int64]fakeRow
	nextVal  int64
	seeded   bool
	errs     map[string]error
	beginErr error
	writes   int

	snapUsers   map[int64]fakeRow
	snapNextVal int64
	snapWrites  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[int64]fakeRow{}, nextVal: 1, seeded: true, errs: map[string]error{}}
}

func copyUsers(m map[int64]fakeRow) map[int64]fakeRow {
	out := make(map[int64]fakeRow, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *fakeStore) put(id int64, name, email, password, role, about any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[id] = fakeRow{id: id, name: name, email: email, password: password, role: role, about: about}
}

func (s *fakeStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

type fakeConnector struct{ s *fakeStore }

func (c *fakeConnector) Connect(context.Context) (driver.Conn, error) { return &fakeConn{s: c.s}, nil }
func (c *fakeConnector) Driver() driver.Driver                       { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type fakeConn struct{ s *fakeStore }

func (c *fakeConn) Prepare(q string) (driver.Stmt, error) { return &fakeStmt{s: c.s, q: q}, nil }
func (c *fakeConn) Close() error                          { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	s.snapUsers = copyUsers(s.users)
	s.snapNextVal = s.nextVal
	s.snapWrites = s.writes
	return &fakeTx{s: s}, nil
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) Commit() error { return nil }
func (t *fakeTx) Rollback() error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.users = t.s.snapUsers
	t.s.nextVal = t.s.snapNextVal
	t.s.writes = t.s.snapWrites
	return nil
}

type fakeStmt struct {
	s *fakeStore
	q string
}

func (st *fakeStmt) Close() error  { return nil }
func (st *fakeStmt) NumInput() int { return -1 }

type fakeResult struct{ n int64 }

func (r fakeResult) LastInsertId() (int64, error) { return 0, errors.New("unsupported") }
func (r fakeResult) RowsAffected() (int64, error) { return r.n, nil }

func (st *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	s := st.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.errs[st.q]; err != nil {
		return nil, err
	}
	switch st.q {
	case updateSQL:
		id := args[0].(int64)
		if _, ok := s.users[id]; !ok {
			return fakeResult{0}, nil
		}
		s.users[id] = fakeRow{id: id, name: args[1], email: args[2], password: args[3], role: args[4], about: args[5]}
		s.writes++
		return fakeResult{1}, nil
	case deleteSQL:
		id := args[0].(int64)
		if _, ok := s.users[id]; !ok {
			return fakeResult{0}, nil
		}
		delete(s.users, id)
		s.writes++
		return fakeResult{1}, nil
	}
	return nil, errors.New("fake: unexpected exec: " + st.q)
}

var userCols = []string{"user_id", "user_name", "user_email", "user_password", "user_role", "user_about"}

func rowVals(r fakeRow) []driver.Value {
	return []driver.Value{r.id, r.name, r.email, r.password, r.role, r.about}
}

func (s *fakeStore) sorted() []fakeRow {
	out := make([]fakeRow, 0, len(s.users))
	for _, r := range s.users {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func (st *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	s := st.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.errs[st.q]; err != nil {
		return nil, err
	}
	switch st.q {
	case selectAllSQL:
		var vals [][]driver.Value
		for _, r := range s.sorted() {
			vals = append(vals, rowVals(r))
		}
		return &fakeRows{cols: userCols, vals: vals}, nil
	case selectPageSQL:
		limit, offset := int(args[0].(int64)), int(args[1].(int64))
		all := s.sorted()
		var vals [][]driver.Value
		for i := offset; i < len(all) && i < offset+limit; i++ {
			vals = append(vals, rowVals(all[i]))
		}
		return &fakeRows{cols: userCols, vals: vals}, nil
	case selectByIDSQL:
		var vals [][]driver.Value
		if r, ok := s.users[args[0].(int64)]; ok {
			vals = append(vals, rowVals(r))
		}
		return &fakeRows{cols: userCols, vals: vals}, nil
	case selectByNameSQL:
		var vals [][]driver.Value
		for _, r := range s.sorted() {
			if n, ok := r.name.(string); ok && n == args[0].(string) && len(vals) < 2 {
				vals = append(vals, rowVals(r))
			}
		}
		return &fakeRows{cols: userCols, vals: vals}, nil
	case nextIDSQL:
		if !s.seeded {
			return &fakeRows{cols: []string{"next_val"}}, nil
		}
		v := s.nextVal
		s.nextVal++
		s.writes++
		return &fakeRows{cols: []string{"next_val"}, vals: [][]driver.Value{{v}}}, nil
	case insertSQL:
		id := args[0].(int64)
		if _, ok := s.users[id]; ok {
			return nil, errors.New("duplicate key user_id")
		}
		if args[2] != nil {
			for _, r := range s.users {
				if r.email == args[2] {
					return nil, errors.New("duplicate key uk_user_user_email")
				}
			}
		}
		s.users[id] = fakeRow{id: id, name: args[1], email: args[2], password: args[3], role: args[4], about: args[5]}
		s.writes++
		return &fakeRows{cols: []string{"user_id"}, vals: [][]driver.Value{{id}}}, nil
	}
	return nil, errors.New("fake: unexpected query: " + st.q)
}

type fakeRows struct {
	cols []string
	vals [][]driver.Value
	i    int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.vals) {
		return io.EOF
	}
	copy(dest, r.vals[r.i])
	r.i++
	return nil
}

func newRepo(t *testing.T, s *fakeStore) *SQLUserRepository {
	t.Helper()
	db := sql.OpenDB(&fakeConnector{s: s})
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLUserRepository(db)
}

func sp(s string) *string { return &s }

func strOrNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func seedHemraj(s *fakeStore) {
	s.put(3, "hemraj", "hemrajmalhi1234@gmail.com", "root", "java developer", "Sr")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFindByName(t *testing.T) {
	queryErr := errors.New("connection lost")
	tests := []struct {
		name    string
		setup   func(s *fakeStore)
		lookup  string
		wantOK  bool
		wantID  int
		wantErr error
		anyErr  bool
	}{
		{name: "existing user hemraj", setup: seedHemraj, lookup: "hemraj", wantOK: true, wantID: 3},
		{name: "no such user", setup: seedHemraj, lookup: "nobody", wantOK: false},
		{name: "exact match only (case sensitive)", setup: seedHemraj, lookup: "Hemraj", wantOK: false},
		{name: "empty table", setup: func(*fakeStore) {}, lookup: "hemraj", wantOK: false},
		{name: "non-unique name", setup: func(s *fakeStore) {
			s.put(1, "dup", "a@x", nil, nil, nil)
			s.put(2, "dup", "b@x", nil, nil, nil)
			s.put(4, "dup", "c@x", nil, nil, nil)
		}, lookup: "dup", wantErr: ErrNonUniqueName},
		{name: "query error", setup: func(s *fakeStore) { s.errs[selectByNameSQL] = queryErr }, lookup: "x", wantErr: queryErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			tc.setup(s)
			before := copyUsers(s.users)
			writes := s.writes
			repo := newRepo(t, s)
			u, ok, err := repo.FindByName(context.Background(), tc.lookup)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if u != nil || ok {
					t.Fatalf("expected no result on error, got %v %v", u, ok)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if ok != tc.wantOK {
					t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
				}
				if ok {
					if u == nil || strOrNil(u.Name) != tc.lookup || u.ID != tc.wantID {
						t.Fatalf("got %v", u)
					}
				} else if u != nil {
					t.Fatalf("expected nil user, got %v", u)
				}
			}
			if s.writes != writes || len(s.users) != len(before) {
				t.Fatalf("FindByName modified data")
			}
		})
	}
}

func TestSave(t *testing.T) {
	beginErr := errors.New("begin failed")
	tests := []struct {
		name      string
		setup     func(s *fakeStore)
		user      *model.User
		wantErr   bool
		wantErrIs error
		wantID    int
		wantCount int
	}{
		{name: "nil user", setup: func(*fakeStore) {}, user: nil, wantErr: true, wantCount: 0},
		{name: "new user gets allocated id", setup: func(*fakeStore) {},
			user: model.NewUserBuilder().Name("alice").Email("a@x").Build(), wantID: 1, wantCount: 1},
		{name: "new user uses sequence value", setup: func(s *fakeStore) { s.nextVal = 10 },
			user: model.NewUserBuilder().Name("bob").Build(), wantID: 10, wantCount: 1},
		{name: "existing id updates", setup: seedHemraj,
			user: model.NewUser(3, sp("hemraj2"), sp("new@x"), nil, nil, nil), wantID: 3, wantCount: 1},
		{name: "unknown non-zero id inserts with new id", setup: func(s *fakeStore) { s.nextVal = 5 },
			user: model.NewUser(99, sp("ghost"), nil, nil, nil, nil), wantID: 5, wantCount: 1},
		{name: "duplicate email rolls back", setup: seedHemraj,
			user: model.NewUserBuilder().Name("other").Email("hemrajmalhi1234@gmail.com").Build(), wantErr: true, wantCount: 1},
		{name: "sequence not seeded", setup: func(s *fakeStore) { s.seeded = false },
			user: model.NewUserBuilder().Name("x").Build(), wantErr: true, wantCount: 0},
		{name: "begin error", setup: func(s *fakeStore) { s.beginErr = beginErr },
			user: model.NewUserBuilder().Name("x").Build(), wantErr: true, wantErrIs: beginErr, wantCount: 0},
		{name: "update exec error", setup: func(s *fakeStore) { seedHemraj(s); s.errs[updateSQL] = errors.New("boom") },
			user: model.NewUser(3, sp("z"), nil, nil, nil, nil), wantErr: true, wantCount: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			tc.setup(s)
			repo := newRepo(t, s)
			var orig model.User
			if tc.user != nil {
				orig = *tc.user
			}
			got, err := repo.Save(context.Background(), tc.user)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
				}
				if got != nil {
					t.Fatalf("expected nil user on error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if got.ID != tc.wantID {
					t.Fatalf("id = %d, want %d", got.ID, tc.wantID)
				}
				if tc.user.ID != orig.ID {
					t.Fatalf("input user mutated")
				}
				byID, ok, err := repo.FindByID(context.Background(), got.ID)
				if err != nil || !ok || !byID.Equal(got) {
					t.Fatalf("FindByID after save: %v %v %v", byID, ok, err)
				}
				if got.Name != nil {
					byName, ok, err := repo.FindByName(context.Background(), *got.Name)
					if err != nil || !ok || !byName.Equal(got) {
						t.Fatalf("FindByName after save: %v %v %v", byName, ok, err)
					}
				}
			}
			if c := s.count(); c != tc.wantCount {
				t.Fatalf("row count = %d, want %d", c, tc.wantCount)
			}
		})
	}
}

func TestFindByID(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *fakeStore)
		id      int
		wantOK  bool
		wantErr bool
	}{
		{name: "exists", setup: seedHemraj, id: 3, wantOK: true},
		{name: "missing", setup: seedHemraj, id: 42},
		{name: "zero id missing", setup: func(*fakeStore) {}, id: 0},
		{name: "query error", setup: func(s *fakeStore) { s.errs[selectByIDSQL] = errors.New("boom") }, id: 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			tc.setup(s)
			writes := s.writes
			repo := newRepo(t, s)
			u, ok, err := repo.FindByID(context.Background(), tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if ok && (u == nil || u.ID != tc.id) {
				t.Fatalf("got %v", u)
			}
			if !ok && u != nil {
				t.Fatalf("expected nil")
			}
			if s.writes != writes {
				t.Fatalf("read modified data")
			}
		})
	}
}

func TestDeleteByID(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(s *fakeStore)
		id        int
		wantErrIs error
		wantErr   bool
	}{
		{name: "existing", setup: seedHemraj, id: 3},
		{name: "missing", setup: seedHemraj, id: 7, wantErrIs: apperror.ErrNotFoundRow, wantErr: true},
		{name: "exec error", setup: func(s *fakeStore) { s.errs[deleteSQL] = errors.New("boom") }, id: 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			tc.setup(s)
			repo := newRepo(t, s)
			err := repo.DeleteByID(context.Background(), tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
			}
			if !tc.wantErr {
				_, ok, err := repo.FindByID(context.Background(), tc.id)
				if err != nil || ok {
					t.Fatalf("user still present after delete")
				}
			}
		})
	}
}

func TestFindAll(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *fakeStore)
		wantIDs []int
		wantErr bool
	}{
		{name: "empty", setup: func(*fakeStore) {}, wantIDs: []int{}},
		{name: "ordered by id", setup: func(s *fakeStore) {
			s.put(5, "e", nil, nil, nil, nil)
			seedHemraj(s)
			s.put(1, "a", nil, nil, nil, nil)
		}, wantIDs: []int{1, 3, 5}},
		{name: "query error", setup: func(s *fakeStore) { s.errs[selectAllSQL] = errors.New("boom") }, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			tc.setup(s)
			writes := s.writes
			repo := newRepo(t, s)
			users, err := repo.FindAll(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if users == nil {
				t.Fatal("FindAll returned nil slice")
			}
			if len(users) != len(tc.wantIDs) {
				t.Fatalf("len = %d, want %d", len(users), len(tc.wantIDs))
			}
			for i, id := range tc.wantIDs {
				if users[i].ID != id {
					t.Fatalf("users[%d].ID = %d, want %d", i, users[i].ID, id)
				}
			}
			if s.writes != writes {
				t.Fatal("read modified data")
			}
		})
	}
}

func TestFindPage(t *testing.T) {
	seed := func(s *fakeStore) {
		for i := int64(1); i <= 5; i++ {
			s.put(i, "u", nil, nil, nil, nil)
		}
	}
	tests := []struct {
		name       string
		page, size int
		wantIDs    []int
		wantErr    bool
	}{
		{name: "first page", page: 0, size: 2, wantIDs: []int{1, 2}},
		{name: "second page", page: 1, size: 2, wantIDs: []int{3, 4}},
		{name: "partial last page", page: 2, size: 2, wantIDs: []int{5}},
		{name: "beyond end", page: 10, size: 2, wantIDs: []int{}},
		{name: "negative page", page: -1, size: 2, wantErr: true},
		{name: "zero size", page: 0, size: 0, wantErr: true},
		{name: "negative size", page: 0, size: -3, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore()
			seed(s)
			repo := newRepo(t, s)
			users, err := repo.FindPage(context.Background(), tc.page, tc.size)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if users == nil || len(users) != len(tc.wantIDs) {
				t.Fatalf("got %v, want ids %v", users, tc.wantIDs)
			}
			for i, id := range tc.wantIDs {
				if users[i].ID != id {
					t.Fatalf("users[%d].ID = %d, want %d", i, users[i].ID, id)
				}
			}
		})
	}
}