package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"database/sql"
)

func sp(s string) *string { return &s }

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

type fakeExecer struct {
	queries []string
	failAt  int
	err     error
}

func (f *fakeExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.queries = append(f.queries, query)
	if f.err != nil && len(f.queries)-1 == f.failAt {
		return nil, f.err
	}
	return fakeResult{}, nil
}

func TestZeroValueUser(t *testing.T) {
	var u User
	if u.ID != 0 || u.Name != nil || u.Email != nil || u.Password != nil || u.Role != nil || u.About != nil {
		t.Fatalf("zero value not empty: %v", &u)
	}
}

func TestNewUser(t *testing.T) {
	tests := []struct {
		name string
		id   int
		n    *string
	}{
		{"full", 1, sp("A")},
		{"blank name accepted", 2, sp("")},
		{"nil name accepted", 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUser(tt.id, tt.n, sp("a@x.com"), sp("p"), sp("ROLE_USER"), sp("hi"))
			if u.ID != tt.id || u.Name != tt.n {
				t.Fatalf("id/name mismatch: %v", u)
			}
			if *u.Email != "a@x.com" || *u.Password != "p" || *u.Role != "ROLE_USER" || *u.About != "hi" {
				t.Fatalf("field order mismatch: %v", u)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	u := NewUserBuilder().Name("A").Email("e").Build()
	if u.ID != 0 || *u.Name != "A" || *u.Email != "e" || u.Password != nil || u.Role != nil || u.About != nil {
		t.Fatalf("unexpected: %v", u)
	}
	b := NewUserBuilder().ID(5).Name("n").Email("e").Password("p").Role("r").About("a")
	full := b.Build()
	b.Name("changed")
	if *full.Name != "n" {
		t.Fatal("builder reuse affected built user")
	}
	if !full.Equal(NewUser(5, sp("n"), sp("e"), sp("p"), sp("r"), sp("a"))) {
		t.Fatalf("unexpected: %v", full)
	}
}

func TestFieldsSetGet(t *testing.T) {
	u := &User{}
	u.ID = 7
	u.Name = sp("  x  ")
	u.Email = sp("E")
	u.Password = sp("P")
	u.Role = sp("R")
	u.About = sp("A")
	if u.ID != 7 || *u.Name != "  x  " || *u.Email != "E" || *u.Password != "P" || *u.Role != "R" || *u.About != "A" {
		t.Fatalf("set/get mismatch: %v", u)
	}
}

func TestEqual(t *testing.T) {
	base := func() *User { return NewUser(1, sp("A"), sp("e"), sp("p"), sp("r"), sp("a")) }
	tests := []struct {
		name  string
		other *User
		want  bool
	}{
		{"identical", base(), true},
		{"diff id", func() *User { u := base(); u.ID = 2; return u }(), false},
		{"diff name", func() *User { u := base(); u.Name = sp("B"); return u }(), false},
		{"nil name", func() *User { u := base(); u.Name = nil; return u }(), false},
		{"diff email", func() *User { u := base(); u.Email = sp("x"); return u }(), false},
		{"diff password", func() *User { u := base(); u.Password = sp("x"); return u }(), false},
		{"diff role", func() *User { u := base(); u.Role = sp("x"); return u }(), false},
		{"diff about", func() *User { u := base(); u.About = nil; return u }(), false},
		{"nil other", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base().Equal(tt.other); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
	var n *User
	if !n.Equal(nil) {
		t.Fatal("nil should equal nil")
	}
	if !(&User{}).Equal(&User{}) {
		t.Fatal("zero users should be equal")
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		u    *User
		want string
	}{
		{"full", NewUser(1, sp("A"), sp("a@x.com"), sp("secret"), sp("ROLE_USER"), sp("hi")),
			"User(id=1, name=A, email=a@x.com, password=secret, role=ROLE_USER, about=hi)"},
		{"zero", &User{}, "User(id=0, name=null, email=null, password=null, role=null, about=null)"},
		{"nil", nil, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.String(); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		n       *string
		wantErr bool
	}{
		{"nil", nil, true},
		{"empty", sp(""), true},
		{"whitespace", sp(" \t\n "), true},
		{"valid", sp("Alice"), false},
		{"padded", sp("  a "), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&User{Name: tt.n}).Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrNameBlank) || err.Error() != "please Add the department Name" {
					t.Fatalf("unexpected err %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	u := NewUser(3, sp("A"), nil, sp("pw"), nil, sp("x"))
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"password":"pw"`) || !strings.Contains(string(data), `"email":null`) {
		t.Fatalf("unexpected json %s", data)
	}
	var back User
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(u) {
		t.Fatalf("roundtrip mismatch %v", &back)
	}
}

func TestMappingConstants(t *testing.T) {
	if TableName != `"user"` {
		t.Fatalf("table not quoted: %s", TableName)
	}
	cols := map[string]string{ColumnID: "user_id", ColumnName: "user_name", ColumnEmail: "user_email",
		ColumnPassword: "user_password", ColumnRole: "user_role", ColumnAbout: "user_about"}
	for got, want := range cols {
		if got != want {
			t.Fatalf("col %s want %s", got, want)
		}
	}
	if NextIDQuery != "SELECT nextval('hibernate_sequence')" {
		t.Fatalf("bad query %s", NextIDQuery)
	}
}

func TestEnsureSchema(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		failAt    int
		err       error
		wantCalls int
	}{
		{"success", -1, nil, 2},
		{"sequence fails", 0, boom, 1},
		{"table fails", 1, boom, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeExecer{failAt: tt.failAt, err: tt.err}
			err := EnsureSchema(context.Background(), f)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("want wrapped err, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(f.queries) != tt.wantCalls {
				t.Fatalf("calls %d want %d", len(f.queries), tt.wantCalls)
			}
			if tt.err == nil {
				ddl := f.queries[1]
				for _, s := range []string{`"user"`, "user_email    VARCHAR(255) UNIQUE", "VARCHAR(500)", "PRIMARY KEY"} {
					if !strings.Contains(ddl, s) {
						t.Fatalf("ddl missing %q", s)
					}
				}
				if strings.Count(ddl, "UNIQUE") != 1 {
					t.Fatal("only email should be unique")
				}
				if !strings.Contains(f.queries[0], "hibernate_sequence") {
					t.Fatal("sequence missing")
				}
			}
		})
	}
}