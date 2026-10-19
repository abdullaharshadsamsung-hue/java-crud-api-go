// Package service implements the application's business operations on top of
// the repository layer. It replaces com.smartContact.service.UserService and
// its Spring @Service implementation UserServiceImp.
//
// Following Go convention, no service interface is declared here: consumers
// (the HTTP layer) declare the small interface they need, and *UserService
// satisfies it implicitly.
package service

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/apperror"
	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

// UserService provides CRUD operations for users. It replaces the Spring
// bean UserServiceImp, whose @Autowired UserDao is now an explicit
// constructor dependency.
type UserService struct {
	repo repository.UserRepository
}

// NewUserService returns a UserService backed by repo.
func NewUserService(repo repository.UserRepository) (*UserService, error) {
	if repo == nil {
		return nil, errors.New("service: new user service: nil repository")
	}
	return &UserService{repo: repo}, nil
}

// SaveUser persists user and returns the saved instance, carrying the
// allocated id (UserServiceImp.saveUser → userDao.save).
func (s *UserService) SaveUser(ctx context.Context, user *model.User) (*model.User, error) {
	saved, err := s.repo.Save(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("service: save user: %w", err)
	}
	return saved, nil
}

// FetchUserList returns every user in the store. The slice is never nil.
func (s *UserService) FetchUserList(ctx context.Context) ([]model.User, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user list: %w", err)
	}
	return users, nil
}

// FetchUserByID returns the user with the given id. When no such user exists
// it returns a *apperror.UserNotFoundError with the message
// "User are not available" (matches errors.Is(err, apperror.ErrUserNotFound)),
// which the HTTP layer maps to 404.
func (s *UserService) FetchUserByID(ctx context.Context, id int) (*model.User, error) {
	u, ok, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user %d: %w", id, err)
	}
	if !ok {
		return nil, apperror.NewUserNotFoundMessage(apperror.UserNotAvailableMessage)
	}
	return u, nil
}

// DeleteUser removes the user with the given id.
//
// MIGRATION_NOTE: as in the source (Spring Data deleteById throwing
// EmptyResultDataAccessException), deleting a missing id is an error, not a
// UserNotFoundException. The returned error wraps apperror.ErrNotFoundRow and
// must NOT be mapped to the 404 ErrorMessage body; the source surfaced it as 500.
func (s *UserService) DeleteUser(ctx context.Context, id int) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("service: delete user %d: %w", id, err)
	}
	return nil
}

// UpdateUser stores user under the given id (UserServiceImp.updateUser:
// user.setId(id); userDao.save(user)). The supplied user's ID is overwritten
// with id, as the source mutated its argument.
//
// MIGRATION_NOTE: JPA merge semantics are preserved by repository.Save: if no
// user with id exists, a new row is inserted with a freshly allocated id
// rather than returning not-found. Fields left nil in user are written as NULL.
func (s *UserService) UpdateUser(ctx context.Context, id int, user *model.User) error {
	if user == nil {
		return errors.New("service: update user: nil user")
	}
	user.ID = id
	if _, err := s.repo.Save(ctx, user); err != nil {
		return fmt.Errorf("service: update user %d: %w", id, err)
	}
	return nil
}

// GetUserNameByName looks up the single user whose name equals name.
// ok is false when no user matches (the source returned null). When several
// users share the name the error wraps repository.ErrNonUniqueName.
func (s *UserService) GetUserNameByName(ctx context.Context, name string) (u *model.User, ok bool, err error) {
	u, ok, err = s.repo.FindByName(ctx, name)
	if err != nil {
		return nil, false, fmt.Errorf("service: get user by name: %w", err)
	}
	return u, ok, nil
}
