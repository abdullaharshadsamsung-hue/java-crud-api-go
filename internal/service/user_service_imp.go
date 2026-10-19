package service

import (
	"context"

	"migrated-app/internal/model"
)

// UserOperations is the full set of user business operations exposed by
// com.smartContact.service.UserServiceImp. *UserService, defined in
// user_service.go, is the only implementation.
//
// Handlers should normally declare the smaller interface they actually need.
// This type exists so consumers that need every operation (for example
// test doubles for the whole controller) can depend on one named contract.
// The compile-time assertion below keeps UserOperations and *UserService in
// sync.
//
// MIGRATION_NOTE: UserServiceImp's behaviour (save, findAll, findById ->
// UserNotFoundException, deleteById, setId+save, findByName) lives on
// *UserService in user_service.go. This file adds no logic of its own, so the
// struct is not declared twice in the package.
type UserOperations interface {
	// SaveUser persists user and returns the saved entity.
	SaveUser(ctx context.Context, user *model.User) (*model.User, error)
	// FetchUserList returns all users.
	FetchUserList(ctx context.Context) ([]model.User, error)
	// FetchUserByID returns the user with id, or an error matching
	// apperror.ErrUserNotFound if no such user exists.
	FetchUserByID(ctx context.Context, id int) (*model.User, error)
	// DeleteUser deletes the user with id.
	DeleteUser(ctx context.Context, id int) error
	// UpdateUser sets id on user and saves it, overwriting the record with
	// that id.
	UpdateUser(ctx context.Context, id int, user *model.User) error
	// GetUserNameByName returns the user whose name exactly equals name.
	// ok is false if there is none (the source returned null).
	GetUserNameByName(ctx context.Context, name string) (*model.User, bool, error)
}

// Compile-time check that *UserService implements UserOperations.
var _ UserOperations = (*UserService)(nil)
