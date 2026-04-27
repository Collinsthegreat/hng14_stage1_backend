package model

import "time"

// User represents an authenticated GitHub user stored in the database.
type User struct {
	ID          string     `json:"id"`
	GithubID    string     `json:"github_id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	AvatarURL   string     `json:"avatar_url"`
	Role        string     `json:"role"` // "admin" | "analyst"
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// RefreshToken represents a stored (hashed) refresh token record.
type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"` // never serialised
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
