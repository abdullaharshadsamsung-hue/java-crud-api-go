// Package model contains the persistent domain types of the application.
//
// It also owns the DDL for those types. OpenDB, which cmd/server/main.go must
// call at startup, opens the database and creates the schema. It replaces
// Hibernate's spring.jpa.hibernate.ddl-auto=update.
package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNameBlank is returned by User.Validate when the name is nil or blank.
// It mirrors the source's @NotBlank(message = "please Add the department Name").
var ErrNameBlank = errors.New("please Add the department Name")

// TableName is the physical table backing User. The source declares
// @Table(name = "USER"), and Spring's physical naming strategy lower-cases it
// to user. It is backtick-quoted because USER is a MySQL keyword.
const TableName = "`user`"

// Column names, the lower-cased equivalents of the source @Column names
// (Spring's physical naming strategy lower-cases identifiers).
const (
	ColumnID       = "user_id"
	ColumnName     = "user_name"
	ColumnEmail    = "user_email"
	ColumnPassword = "user_password"
	ColumnRole     = "user_role"
	ColumnAbout    = "user_about"
)

// IDSequenceTable is the table Hibernate uses to allocate ids for
// GenerationType.AUTO on MySQL, which has no native sequences.
// It holds a single row with a next_val column.
const IDSequenceTable = "hibernate_sequence"

// schemaStatements reproduces the DDL that Hibernate ddl-auto=update emits
// for this entity with MySQL8Dialect (the dialect in application.properties).
//
// Every statement is idempotent, so running them on each boot is safe.
var schemaStatements = []string{
	"CREATE TABLE IF NOT EXISTS " + TableName + " (" +
		"user_id INTEGER NOT NULL, " +
		"user_name VARCHAR(255), " +
		"user_email VARCHAR(255), " +
		"user_password VARCHAR(255), " +
		"user_role VARCHAR(255), " +
		"user_about VARCHAR(500), " +
		"PRIMARY KEY (user_id), " +
		"CONSTRAINT uk_user_user_email UNIQUE (user_email)" +
		") ENGINE=InnoDB",
	"CREATE TABLE IF NOT EXISTS " + IDSequenceTable + " (next_val BIGINT) ENGINE=InnoDB",
	// Hibernate seeds the sequence table with 1. Only do so when it is empty,
	// so that restarts never reset the counter.
	"INSERT INTO " + IDSequenceTable + " (next_val) SELECT 1 FROM DUAL " +
		"WHERE NOT EXISTS (SELECT 1 FROM " + IDSequenceTable + ")",
}

// Execer is the minimal database capability needed to create the schema.
// *sql.DB, *sql.Tx and *sql.Conn all satisfy it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Querier can both execute statements and read a single row.
// *sql.DB, *sql.Tx and *sql.Conn all satisfy it.
type Querier interface {
	Execer
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// EnsureSchema creates the User table and the hibernate_sequence id table if
// they do not already exist, and seeds the sequence. It is called by OpenDB.
// Call it directly only if you open the *sql.DB yourself.
func EnsureSchema(ctx context.Context, db Execer) error {
	for _, stmt := range schemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("model: ensure user schema: %w", err)
		}
	}
	return nil
}

// OpenDB opens the database, verifies connectivity and creates the schema.
// It is the boot-time replacement for Spring's DataSource plus Hibernate's
// ddl-auto=update, and cmd/server/main.go must call it at startup.
//
// The caller must import the driver, e.g. _ "github.com/go-sql-driver/mysql",
// and close the returned *sql.DB on shutdown.
func OpenDB(ctx context.Context, driverName, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, errors.New("model: open database: empty DSN")
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("model: open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("model: ping database: %w", err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// NextID allocates the next User id from hibernate_sequence, the way
// Hibernate's table-backed AUTO generator does on MySQL.
//
// It must be called inside a transaction (pass the *sql.Tx), because
// SELECT ... FOR UPDATE only serialises concurrent callers while a
// transaction is open.
func NextID(ctx context.Context, q Querier) (int, error) {
	var next int64
	err := q.QueryRowContext(ctx, "SELECT next_val FROM "+IDSequenceTable+" FOR UPDATE").Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("model: read next user id: %w", err)
	}
	if _, err := q.ExecContext(ctx, "UPDATE "+IDSequenceTable+" SET next_val = ? WHERE next_val = ?", next+1, next); err != nil {
		return 0, fmt.Errorf("model: advance user id sequence: %w", err)
	}
	return int(next), nil
}

// User is the application user. Nullable string columns are pointers, so SQL
// NULL and JSON null round-trip faithfully, as with Java's String fields.
// The zero value is the equivalent of the source's no-args constructor.
//
// MIGRATION_NOTE: Lombok getters/setters are replaced by exported fields.
// The password is intentionally still serialized, matching the source behaviour.
type User struct {
	ID       int     `json:"id"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
	About    *string `json:"about"`
}

// NewUser builds a User from every field, in declaration order
// (Lombok @AllArgsConstructor).
func NewUser(id int, name, email, password, role, about *string) *User {
	return &User{
		ID:       id,
		Name:     name,
		Email:    email,
		Password: password,
		Role:     role,
		About:    about,
	}
}

// Validate enforces the Bean Validation constraint of the source entity:
// name must be non-nil and contain at least one non-whitespace character.
func (u *User) Validate() error {
	if u.Name == nil || strings.TrimSpace(*u.Name) == "" {
		return ErrNameBlank
	}
	return nil
}

// Equal reports whether u and other hold the same values in all six fields
// (Lombok @EqualsAndHashCode).
func (u *User) Equal(other *User) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.ID == other.ID &&
		strPtrEqual(u.Name, other.Name) &&
		strPtrEqual(u.Email, other.Email) &&
		strPtrEqual(u.Password, other.Password) &&
		strPtrEqual(u.Role, other.Role) &&
		strPtrEqual(u.About, other.About)
}

// String renders the user in Lombok's toString format:
// User(id=..., name=..., email=..., password=..., role=..., about=...).
func (u *User) String() string {
	if u == nil {
		return "null"
	}
	var b strings.Builder
	b.WriteString("User(id=")
	b.WriteString(strconv.Itoa(u.ID))
	b.WriteString(", name=")
	b.WriteString(strPtrString(u.Name))
	b.WriteString(", email=")
	b.WriteString(strPtrString(u.Email))
	b.WriteString(", password=")
	b.WriteString(strPtrString(u.Password))
	b.WriteString(", role=")
	b.WriteString(strPtrString(u.Role))
	b.WriteString(", about=")
	b.WriteString(strPtrString(u.About))
	b.WriteString(")")
	return b.String()
}

// UserBuilder is a fluent builder for User (Lombok @Builder).
type UserBuilder struct {
	u User
}

// NewUserBuilder returns an empty UserBuilder.
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{}
}

// ID sets the id.
func (b *UserBuilder) ID(id int) *UserBuilder {
	b.u.ID = id
	return b
}

// Name sets the name.
func (b *UserBuilder) Name(name string) *UserBuilder {
	b.u.Name = &name
	return b
}

// Email sets the email.
func (b *UserBuilder) Email(email string) *UserBuilder {
	b.u.Email = &email
	return b
}

// Password sets the password.
func (b *UserBuilder) Password(password string) *UserBuilder {
	b.u.Password = &password
	return b
}

// Role sets the role.
func (b *UserBuilder) Role(role string) *UserBuilder {
	b.u.Role = &role
	return b
}

// About sets the about text.
func (b *UserBuilder) About(about string) *UserBuilder {
	b.u.About = &about
	return b
}

// Build returns a new User holding the builder's values. The builder may be
// reused afterwards without affecting the returned User.
func (b *UserBuilder) Build() *User {
	u := b.u
	return &u
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func strPtrString(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}
