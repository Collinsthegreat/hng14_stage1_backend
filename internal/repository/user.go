package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository defines persistence operations for users and refresh tokens.
type UserRepository interface {
	UpsertUser(ctx context.Context, u *model.User) error
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	InvalidateRefreshToken(ctx context.Context, tokenHash string) error
	InvalidateUserRefreshTokens(ctx context.Context, userID string) error
}

type userRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

// UpsertUser inserts or updates a user record keyed by github_id.
func (r *userRepository) UpsertUser(ctx context.Context, u *model.User) error {
	query := `
		INSERT INTO users (id, github_id, username, email, avatar_url, role, is_active, last_login_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (github_id) DO UPDATE SET
			username     = EXCLUDED.username,
			email        = EXCLUDED.email,
			avatar_url   = EXCLUDED.avatar_url,
			last_login_at = EXCLUDED.last_login_at
		RETURNING id, role, is_active, created_at
	`
	return r.pool.QueryRow(ctx, query,
		u.ID, u.GithubID, u.Username, u.Email, u.AvatarURL,
		u.Role, u.IsActive, u.LastLoginAt, u.CreatedAt,
	).Scan(&u.ID, &u.Role, &u.IsActive, &u.CreatedAt)
}

// GetUserByID fetches a user by primary key.
func (r *userRepository) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	query := `
		SELECT id, github_id, username, email, avatar_url, role, is_active, last_login_at, created_at
		FROM users WHERE id = $1
	`
	var u model.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.GithubID, &u.Username, &u.Email, &u.AvatarURL,
		&u.Role, &u.IsActive, &u.LastLoginAt, &u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// SaveRefreshToken invalidates any prior tokens for the user, then saves the new hashed token.
// One valid refresh token per user at a time.
func (r *userRepository) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Invalidate all existing tokens for this user
	_, err = tx.Exec(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE user_id = $1 AND used = FALSE`, userID)
	if err != nil {
		return fmt.Errorf("invalidate old tokens: %w", err)
	}

	// Insert the new token
	id, genErr := generateID()
	if genErr != nil {
		return genErr
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		id, userID, tokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}

	return tx.Commit(ctx)
}

// GetRefreshTokenByHash retrieves a refresh token record by its SHA-256 hash.
func (r *userRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`
	var rt model.RefreshToken
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.Used, &rt.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

// InvalidateRefreshToken marks a single refresh token as used.
func (r *userRepository) InvalidateRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE token_hash = $1`, tokenHash)
	return err
}

// InvalidateUserRefreshTokens marks all refresh tokens for a user as used (logout).
func (r *userRepository) InvalidateUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE user_id = $1`, userID)
	return err
}

// generateID returns a new UUID v7 string.
func generateID() (string, error) {
	// Reuse the uuid package already in the module
	return newUUIDv7()
}
