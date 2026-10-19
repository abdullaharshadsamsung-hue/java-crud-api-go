// Package repository provides persistence access for the application's
// domain types. It replaces the Spring Data JPA repository interfaces in
// com.smartContact.repository.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
)

// ErrNonUniqueName is returned by FindByName when more than one user has the
// requested name. It mirrors Spring's IncorrectResultSizeDataAccessException,
// which the source surfaces as a 500.
var ErrNonUniqueName = errors.New("repository: query did not return a unique result")

// UserRepository is the Go equivalent of UserDao extends JpaRepository<User,Integer>.
// It exposes the inherited CRUD operations the application uses plus the
// derived query findByName.
type UserRepository interface {
	// Save inserts u when it is new and otherwise updates every column (JPA
	// save/merge). It returns the persisted user, carrying the allocated id.
	Save(ctx context.Context, u *model.User) (*model.User, error)
	// FindAll returns every user. The result is never nil.
	FindAll(ctx context.Context) ([]model.User, error)
	// FindPage returns one page of users ordered by id (JpaRepository.findAll(Pageable)).
	FindPage(ctx context.Context, page, size int) ([]model.User, error)
	// FindByID looks up a user by primary key. ok is false when none exists.
	FindByID(ctx context.Context, id int) (u *model.User, ok bool, err error)
	// FindByName looks up the single user whose name exactly equals name.
	// ok is false when none exists; ErrNonUniqueName is returned when several match.
	FindByName(ctx context.Context, name string) (u *model.User, ok bool, err error)
	// DeleteByID removes the user with the given id. It returns
	// apperror.ErrNotFoundRow when no row was deleted.
	DeleteByID(ctx context.Context, id int) error
}

// PostgreSQL identifiers. "user" is a reserved word in PostgreSQL, so it is
// double-quoted.
//
// MIGRATION_NOTE: model.TableName is backtick-quoted (MySQL syntax) and
// model.NextID uses '?' placeholders; neither works on PostgreSQL, so this
// repository uses its own PostgreSQL-safe table name and id allocation.
// model.EnsureSchema's DDL (ENGINE=InnoDB, SELECT ... FROM DUAL) must also be
// ported to PostgreSQL — manual review required.
const (
	userTable   = `"user"`
	seqTable    = model.IDSequenceTable
	userColumns = model.ColumnID + ", " + model.ColumnName + ", " + model.ColumnEmail + ", " +
		model.ColumnPassword + ", " + model.ColumnRole + ", " + model.ColumnAbout
)

var (
	selectAllSQL  = "SELECT " + userColumns + " FROM " + userTable + " ORDER BY " + model.ColumnID
	selectPageSQL = "SELECT " + userColumns + " FROM " + userTable + " ORDER BY " + model.ColumnID + " LIMIT $1 OFFSET $2"
	selectByIDSQL = "SELECT " + userColumns + " FROM " + userTable + " WHERE " + model.ColumnID + " = $1"
	// LIMIT 2 is enough to detect a non-unique result without reading the table.
	selectByNameSQL = "SELECT " + userColumns + " FROM " + userTable + " WHERE " + model.ColumnName + " = $1 LIMIT 2"
	insertSQL       = "INSERT INTO " + userTable + " (" + userColumns + ") VALUES ($1, $2, $3, $4, $5, $6) RETURNING " + model.ColumnID
	updateSQL       = "UPDATE " + userTable + " SET " +
		model.ColumnName + " = $2, " +
		model.ColumnEmail + " = $3, " +
		model.ColumnPassword + " = $4, " +
		model.ColumnRole + " = $5, " +
		model.ColumnAbout + " = $6 WHERE " + model.ColumnID + " = $1"
	deleteSQL = "DELETE FROM " + userTable + " WHERE " + model.ColumnID + " = $1"
	// Atomically allocates the next id from the Hibernate-style sequence table.
	// The UPDATE takes a row lock, serialising concurrent allocators.
	nextIDSQL = "UPDATE " + seqTable + " SET next_val = next_val + 1 RETURNING next_val - 1"
)

// SQLUserRepository is the database/sql implementation of UserRepository
// targeting PostgreSQL.
type SQLUserRepository struct {
	db *sql.DB
}

// Compile-time check.
var _ UserRepository = (*SQLUserRepository)(nil)

// NewSQLUserRepository returns a UserRepository backed by db.
func NewSQLUserRepository(db *sql.DB) *SQLUserRepository {
	return &SQLUserRepository{db: db}
}

// Save reproduces JPA save/merge semantics in a single transaction:
//   - id == 0: allocate an id from hibernate_sequence and INSERT;
//   - existing id: UPDATE every column (nil fields become NULL);
//   - non-zero id with no row: allocate a new id and INSERT, ignoring the
//     supplied id (as merge does for a detached entity with an unknown id).
//
// Constraint violations (e.g. duplicate email) are returned wrapped.
func (r *SQLUserRepository) Save(ctx context.Context, u *model.User) (saved *model.User, err error) {
	if u == nil {
		return nil, errors.New("repository: save user: nil user")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("repository: save user: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	out := *u
	if out.ID != 0 {
		res, err := tx.ExecContext(ctx, updateSQL, out.ID, out.Name, out.Email, out.Password, out.Role, out.About)
		if err != nil {
			return nil, fmt.Errorf("repository: update user %d: %w", out.ID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("repository: update user %d: rows affected: %w", out.ID, err)
		}
		if n > 0 {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("repository: update user %d: commit: %w", out.ID, err)
			}
			return &out, nil
		}
		// Unknown id: fall through to insert with a freshly allocated id.
	}

	id, err := nextID(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, insertSQL, id, out.Name, out.Email, out.Password, out.Role, out.About).Scan(&out.ID); err != nil {
		return nil, fmt.Errorf("repository: insert user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repository: insert user: commit: %w", err)
	}
	return &out, nil
}

func nextID(ctx context.Context, tx *sql.Tx) (int, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, nextIDSQL).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("repository: allocate user id: %s is not seeded", seqTable)
		}
		return 0, fmt.Errorf("repository: allocate user id: %w", err)
	}
	return int(id), nil
}

// FindAll returns every user ordered by id. It never returns a nil slice.
func (r *SQLUserRepository) FindAll(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, selectAllSQL)
	if err != nil {
		return nil, fmt.Errorf("repository: find all users: %w", err)
	}
	return scanUsers(rows)
}

// FindPage returns the zero-based page of users of the given size, ordered by id.
func (r *SQLUserRepository) FindPage(ctx context.Context, page, size int) ([]model.User, error) {
	if page < 0 {
		return nil, fmt.Errorf("repository: find users page: page index must not be negative, got %d", page)
	}
	if size < 1 {
		return nil, fmt.Errorf("repository: find users page: page size must be positive, got %d", size)
	}
	rows, err := r.db.QueryContext(ctx, selectPageSQL, size, page*size)
	if err != nil {
		return nil, fmt.Errorf("repository: find users page: %w", err)
	}
	return scanUsers(rows)
}

// FindByID returns the user with the given id, or ok=false if none exists.
func (r *SQLUserRepository) FindByID(ctx context.Context, id int) (*model.User, bool, error) {
	var u model.User
	err := scanUser(r.db.QueryRowContext(ctx, selectByIDSQL, id), &u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("repository: find user %d: %w", id, err)
	}
	return &u, true, nil
}

// FindByName returns the single user whose name exactly equals name
// (derived query findByName → WHERE user_name = $1). It returns ok=false when
// none exists and ErrNonUniqueName when more than one matches.
func (r *SQLUserRepository) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	rows, err := r.db.QueryContext(ctx, selectByNameSQL, name)
	if err != nil {
		return nil, false, fmt.Errorf("repository: find user by name: %w", err)
	}
	users, err := scanUsers(rows)
	if err != nil {
		return nil, false, fmt.Errorf("repository: find user by name: %w", err)
	}
	switch len(users) {
	case 0:
		return nil, false, nil
	case 1:
		return &users[0], true, nil
	default:
		return nil, false, fmt.Errorf("repository: find user by name %q: %w", name, ErrNonUniqueName)
	}
}

// DeleteByID removes the user with the given id. It returns
// apperror.ErrNotFoundRow (wrapped) when no such user exists.
func (r *SQLUserRepository) DeleteByID(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx, deleteSQL, id)
	if err != nil {
		return fmt.Errorf("repository: delete user %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: delete user %d: rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("repository: delete user %d: %w", id, apperror.ErrNotFoundRow)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(s rowScanner, u *model.User) error {
	return s.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Role, &u.About)
}

func scanUsers(rows *sql.Rows) (users []model.User, err error) {
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	users = make([]model.User, 0)
	for rows.Next() {
		var u model.User
		if err := scanUser(rows, &u); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}
