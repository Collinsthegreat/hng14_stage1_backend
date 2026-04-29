package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/client"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ─── JWT Claims ────────────────────────────────────────────────────────────────

// Claims is the JWT payload used for access tokens.
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// ─── State Store ───────────────────────────────────────────────────────────────

// stateEntry holds a PKCE code_verifier.
type stateEntry struct {
	codeVerifier string
	expiresAt    time.Time
}

// stateStore is an in-memory store for OAuth state values with TTL.
type stateStore struct {
	mu sync.Map
}

// set stores a state entry with a 10-minute TTL.
func (s *stateStore) set(state, codeVerifier string) {
	s.mu.Store(state, stateEntry{
		codeVerifier: codeVerifier,
		expiresAt:     time.Now().Add(10 * time.Minute),
	})
}

// get retrieves and deletes a state entry, returning false if missing or expired.
func (s *stateStore) get(state string) (stateEntry, bool) {
	v, ok := s.mu.LoadAndDelete(state)
	if !ok {
		return stateEntry{}, false
	}
	entry := v.(stateEntry)
	if time.Now().After(entry.expiresAt) {
		return stateEntry{}, false
	}
	return entry, true
}

// startCleanup launches a background goroutine that purges expired states every 60 seconds.
func (s *stateStore) startCleanup() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			s.mu.Range(func(k, v any) bool {
				if now.After(v.(stateEntry).expiresAt) {
					s.mu.Delete(k)
				}
				return true
			})
		}
	}()
}

// ─── AuthService ───────────────────────────────────────────────────────────────

// AuthService handles GitHub OAuth, JWT issuance, and refresh token lifecycle.
type AuthService interface {
	BuildGitHubAuthURL(state, codeChallenge, codeChallengeMethod, redirectURI string) string
	StoreState(state, codeVerifier string)
	ValidateAndPopState(state string) (codeVerifier string, ok bool)
	HandleCallback(ctx context.Context, code, state, codeVerifier string) (*model.User, string, string, error)
	IssueTokenPair(ctx context.Context, user *model.User) (accessToken string, refreshToken string, err error)
	RefreshTokens(ctx context.Context, rawRefreshToken string) (accessToken string, newRefreshToken string, err error)
	Logout(ctx context.Context, rawRefreshToken string) error
}

type authService struct {
	userRepo    repository.UserRepository
	githubCli   client.GitHubClient
	jwtSecret   []byte
	states      stateStore
	githubRedirectURI string
}

// NewAuthService creates a new AuthService and starts the state cleanup goroutine.
func NewAuthService(userRepo repository.UserRepository, ghCli client.GitHubClient) AuthService {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		slog.Error("JWT_SECRET env var is not set — auth will be insecure")
	}
	svc := &authService{
		userRepo:          userRepo,
		githubCli:         ghCli,
		jwtSecret:         []byte(secret),
		githubRedirectURI: os.Getenv("GITHUB_REDIRECT_URI"),
	}
	svc.states.startCleanup()
	return svc
}

// BuildGitHubAuthURL constructs the GitHub OAuth redirect URL.
func (s *authService) BuildGitHubAuthURL(state, codeChallenge, codeChallengeMethod, redirectURI string) string {
	clientID := os.Getenv("GITHUB_CLIENT_ID")
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("state", state)
	if codeChallenge != "" {
		params.Set("code_challenge", codeChallenge)
		params.Set("code_challenge_method", codeChallengeMethod)
	}
	// Use the CLI's redirect_uri if provided, else the default backend callback.
	callbackURI := s.githubRedirectURI
	if redirectURI != "" {
		callbackURI = redirectURI
	}
	// GitHub always needs the redirect_uri to match what's registered.
	// We redirect to the backend callback regardless and let it proxy.
	_ = callbackURI
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

// StoreState saves a state → codeVerifier mapping with 10-minute TTL.
func (s *authService) StoreState(state, codeVerifier string) {
	s.states.set(state, codeVerifier)
}

// ValidateAndPopState retrieves and removes a state entry.
func (s *authService) ValidateAndPopState(state string) (string, bool) {
	entry, ok := s.states.get(state)
	if !ok {
		return "", false
	}
	return entry.codeVerifier, true
}

// HandleCallback validates the OAuth callback, exchanges the code, upserts the user, and issues tokens.
func (s *authService) HandleCallback(ctx context.Context, code, state, codeVerifier string) (*model.User, string, string, error) {
	// Exchange code for GitHub access token
	ghToken, err := s.githubCli.ExchangeCode(ctx, code, codeVerifier, s.githubRedirectURI)
	if err != nil {
		return nil, "", "", fmt.Errorf("github code exchange: %w", err)
	}

	// Fetch GitHub user info
	ghUser, err := s.githubCli.GetUser(ctx, ghToken)
	if err != nil {
		return nil, "", "", fmt.Errorf("github get user: %w", err)
	}

	// Upsert user
	now := time.Now().UTC()
	userID, _ := uuid.NewV7()
	u := &model.User{
		ID:          userID.String(),
		GithubID:    fmt.Sprintf("%d", ghUser.ID),
		Username:    ghUser.Login,
		Email:       ghUser.Email,
		AvatarURL:   ghUser.AvatarURL,
		Role:        "analyst",
		IsActive:    true,
		LastLoginAt: &now,
		CreatedAt:   now,
	}
	if err := s.userRepo.UpsertUser(ctx, u); err != nil {
		return nil, "", "", fmt.Errorf("upsert user: %w", err)
	}

	// Issue token pair
	accessToken, refreshToken, err := s.IssueTokenPair(ctx, u)
	if err != nil {
		return nil, "", "", err
	}
	return u, accessToken, refreshToken, nil
}

// IssueTokenPair creates a JWT access token (3min) and an opaque refresh token (5min, hashed in DB).
func (s *authService) IssueTokenPair(ctx context.Context, u *model.User) (string, string, error) {
	// Access token — JWT, 3 minutes
	now := time.Now()
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(3 * time.Minute)),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}

	// Refresh token — opaque random, store SHA-256 hash in DB
	rawRefresh, err := generateOpaqueToken()
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	tokenHash := hashToken(rawRefresh)
	expiresAt := now.Add(5 * time.Minute)

	if err := s.userRepo.SaveRefreshToken(ctx, u.ID, tokenHash, expiresAt); err != nil {
		return "", "", fmt.Errorf("save refresh token: %w", err)
	}
	return accessToken, rawRefresh, nil
}

// RefreshTokens validates the raw refresh token, rotates it, and issues a new pair.
func (s *authService) RefreshTokens(ctx context.Context, rawRefreshToken string) (string, string, error) {
	tokenHash := hashToken(rawRefreshToken)

	rt, err := s.userRepo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return "", "", fmt.Errorf("lookup refresh token: %w", err)
	}
	if rt == nil || rt.Used || time.Now().After(rt.ExpiresAt) {
		return "", "", &AuthError{Message: "invalid or expired refresh token"}
	}

	// Mark used immediately before issuing new pair (rotation)
	if err := s.userRepo.InvalidateRefreshToken(ctx, tokenHash); err != nil {
		return "", "", fmt.Errorf("invalidate old token: %w", err)
	}

	// Fetch user for new claims
	u, err := s.userRepo.GetUserByID(ctx, rt.UserID)
	if err != nil || u == nil {
		return "", "", fmt.Errorf("user not found for refresh: %w", err)
	}
	if !u.IsActive {
		return "", "", &AuthError{Message: "account disabled"}
	}

	return s.IssueTokenPair(ctx, u)
}

// Logout invalidates all refresh tokens for the user identified by the raw refresh token.
func (s *authService) Logout(ctx context.Context, rawRefreshToken string) error {
	tokenHash := hashToken(rawRefreshToken)
	rt, err := s.userRepo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return err
	}
	if rt == nil {
		return nil // already invalid — treat as success
	}
	return s.userRepo.InvalidateUserRefreshTokens(ctx, rt.UserID)
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

// AuthError is a sentinel error type for authentication failures.
type AuthError struct{ Message string }

func (e *AuthError) Error() string { return e.Message }
func IsAuthError(err error) bool {
	_, ok := err.(*AuthError)
	return ok
}

// generateOpaqueToken creates a cryptographically secure random token (32 bytes hex).
func generateOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex digest of a token string.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GenerateState creates a cryptographically secure random state string (32 bytes hex).
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateCodeVerifier generates a high-entropy PKCE code verifier.
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateCodeChallenge derives the S256 code challenge from a verifier.
func GenerateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// ParseAndValidateJWT validates a signed JWT string and returns the claims.
func ParseAndValidateJWT(tokenString string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
