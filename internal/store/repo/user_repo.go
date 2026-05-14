package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
)

type userRepo struct {
	q *queries.Queries
}

func NewUserRepo(db *sql.DB) domain.UserRepository {
	return &userRepo{q: queries.New(db)}
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return mapUser(user), nil
}

func (r *userRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	user, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return mapUser(user), nil
}

func (r *userRepo) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	created, err := r.q.CreateUser(ctx, &queries.User{
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
	})
	if err != nil {
		return nil, err
	}
	return mapUser(created), nil
}

func (r *userRepo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteUser(ctx, id)
}

func (r *userRepo) UpdateLastLogin(ctx context.Context, id int64) error {
	return r.q.UpdateLastLogin(ctx, id)
}

func (r *userRepo) List(ctx context.Context) ([]*domain.User, error) {
	users, err := r.q.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.User, len(users))
	for i, u := range users {
		result[i] = mapUser(&u)
	}
	return result, nil
}

func mapUser(u *queries.User) *domain.User {
	user := &domain.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		CreatedAt:    parseTime(u.CreatedAt),
	}
	if u.LastLogin.Valid {
		if t := parseTimePtr(u.LastLogin.String); t != nil {
			user.LastLogin = t
		}
	}
	return user
}

func parseTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseTimePtr(s string) *time.Time {
	t := parseTime(s)
	if t.IsZero() {
		return nil
	}
	return &t
}