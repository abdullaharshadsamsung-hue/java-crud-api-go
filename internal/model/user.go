// Package model contains the persistent domain types of the application.
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

// TableName is the physical table backing User (source: @Table(name = "USER")).
// "user" is a reserved word in PostgreSQL, so it must always be double-quoted.
const TableName = `"user"`

// Column names, the lower-cased equivalents of the source @Column names.
const (
	ColumnID       = "user_id"
	ColumnName     = "user_name"
	ColumnEmail    = "user_email"
	ColumnPassword = "user_password"
	ColumnRole     = "user_role"
	ColumnAbout    = "user_about"
)

// IDSequence is the sequence used to allocate User ids, mirroring Hibernate's
// GenerationType.AUTO default (hibernate_sequence).
const IDSequence = "hibernate_sequence"

// NextIDQuery fetches the next id from IDSequence (PostgreSQL).
const NextIDQuery = `SELECT nextval('` + IDSequence + `')`

// schemaStatements recreates what Hibernate ddl-auto=update derived from the
// @Entity mapping. The id column is a plain INT primary key (not an identity
// column) because ids are allocated from hibernate_sequence, as with AUTO.
var schemaStatements = []string{
	`CREATE SEQUENCE IF NOT EXISTS ` + IDSequence + ` START WITH 1 INCREMENT BY 1`,
	`CREATE TABLE IF NOT EXISTS ` + TableName + ` (
	user_id       INTEGER      NOT NULL PRIMARY KEY,
	user_name     VARCHAR(255),
	user_email    VARCHAR(255) UNIQUE,
	user_password VARCHAR(255),
	user_role     VARCHAR(255),
	user_about    VARCHAR(500)
)`,
}

// Execer is the minimal database capability needed to create the schema.
// *sql.DB, *sql.Tx and *sql.Conn all satisfy it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// EnsureSchema creates the User table and its id sequence if they do not
// already exist. Call it once at application startup (replaces Hibernate's
// ddl-auto=update for this entity).
func EnsureSchema(ctx context.Context, db Execer) error {
	for _, stmt := range schemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("model: ensure user schema: %w", err)
		}
	}
	return nil
}

// User is the application user. Nullable string columns are pointers so that
// SQL NULL and JSON null round-trip faithfully, as with Java's String fields.
// The zero value is the equivalent of the source's no-args constructor.
//
// MIGRATION_NOTE: Lombok getters/setters are replaced by exported fields; the
// password is intentionally still serialized, matching the source behaviour.
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

// Validate enforces the Bean Validation constraints of the source entity:
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
