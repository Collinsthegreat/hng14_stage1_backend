package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/middleware"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/service"
	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

// AuthHandler handles all GitHub OAuth and token lifecycle endpoints.
type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// RedirectToGitHub handles GET /auth/github.
// Detects CLI flow by the presence of the code_challenge query param.
// Generates state, stores it, and redirects to GitHub OAuth.
func (h *AuthHandler) RedirectToGitHub(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	incomingState := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := ""
	if codeChallenge != "" {
		codeChallengeMethod = "S256"
	}
	redirectURI := q.Get("redirect_uri")
	isCLI := codeChallenge != ""

	// Generate or use the provided state
	var state string
	var err error
	if incomingState != "" {
		state = incomingState
	} else {
		state, err = service.GenerateState()
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	// Store state with the original PKCE code_challenge for callback validation.
	h.svc.StoreState(state, codeChallenge, redirectURI, isCLI)

	githubURL := h.svc.BuildGitHubAuthURL(state, codeChallenge, codeChallengeMethod, redirectURI)
	http.Redirect(w, r, githubURL, http.StatusFound)
}

// HandleCallback handles GET /auth/github/callback.
// - Validates state
// - Exchanges code (+ PKCE verifier) for tokens
// - Upserts user
// - CLI: returns JSON to redirect_uri; Browser: sets HTTP-only cookies and redirects
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	codeVerifier := q.Get("code_verifier")

	if code == "" || state == "" {
		response.Error(w, http.StatusBadRequest, "missing code or state")
		return
	}

	storedState, err := h.svc.ValidateAndPopState(state, codeVerifier)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCodeVerifier) {
			response.Error(w, http.StatusBadRequest, "invalid code verifier")
			return
		}
		response.Error(w, http.StatusBadRequest, "invalid state")
		return
	}

	user, accessToken, refreshToken, err := h.svc.HandleCallback(r.Context(), code, state, codeVerifier)
	if err != nil {
		slog.Error("auth callback error", "error", err)
		response.Error(w, http.StatusInternalServerError, "authentication failed")
		return
	}

	if storedState.IsCLI {
		payload := map[string]interface{}{
			"status":        "success",
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"username":      user.Username,
		}
		if storedState.RedirectURI != "" {
			if err := deliverCLIAuthPayload(storedState.RedirectURI, payload); err != nil {
				slog.Warn("cli callback delivery failed", "error", err, "redirect_uri", storedState.RedirectURI)
				if codeVerifier == "" {
					callbackURL, parseErr := url.Parse(storedState.RedirectURI)
					if parseErr != nil {
						response.Error(w, http.StatusInternalServerError, "invalid CLI redirect URI")
						return
					}
					cq := callbackURL.Query()
					cq.Set("access_token", accessToken)
					cq.Set("refresh_token", refreshToken)
					cq.Set("username", user.Username)
					cq.Set("state", state)
					callbackURL.RawQuery = cq.Encode()
					http.Redirect(w, r, callbackURL.String(), http.StatusFound)
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
		return
	}
	if storedState.RedirectURI != "" {
		callbackURL, err := url.Parse(storedState.RedirectURI)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "invalid CLI redirect URI")
			return
		}
		cq := callbackURL.Query()
		cq.Set("access_token", accessToken)
		cq.Set("refresh_token", refreshToken)
		cq.Set("username", user.Username)
		cq.Set("state", state)
		callbackURL.RawQuery = cq.Encode()
		http.Redirect(w, r, callbackURL.String(), http.StatusFound)
		return
	}

	// Browser flow: set HTTP-only cookies and redirect to dashboard
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = ""
	}

	setAuthCookies(w, accessToken, refreshToken)
	dashboardURL := frontendURL + "/dashboard"
	if frontendURL == "" {
		dashboardURL = "/dashboard"
	}
	redirectURL, err := url.Parse(dashboardURL)
	if err != nil {
		slog.Error("invalid frontend redirect url", "error", err, "url", dashboardURL)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	query := redirectURL.Query()
	query.Set("access_token", accessToken)
	query.Set("refresh_token", refreshToken)
	query.Set("username", user.Username)
	redirectURL.RawQuery = query.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// Refresh handles POST /auth/refresh.
// Does NOT require JWT auth — called when access token is expired.
// Reads refresh_token from JSON body, rotates the pair.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		response.Error(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	accessToken, newRefreshToken, err := h.svc.RefreshTokens(r.Context(), body.RefreshToken)
	if err != nil {
		if service.IsAuthError(err) {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		slog.Error("token refresh error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"status":        "success",
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

// Logout handles POST /auth/logout.
// Does NOT require JWT auth — reads refresh_token from body, invalidates it.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		response.Error(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	if err := h.svc.Logout(r.Context(), body.RefreshToken); err != nil {
		if service.IsAuthError(err) {
			response.Error(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		slog.Error("logout error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Clear cookies (browser clients)
	clearAuthCookies(w)

	response.JSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "logged out",
	})
}

// Me handles GET /api/users/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.svc.GetUserByID(r.Context(), userID)
	if err != nil {
		slog.Error("me lookup error", "error", err, "user_id", userID)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if user == nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"data":   userSelfResponse(user),
	})
}

// ─── Cookie helpers ────────────────────────────────────────────────────────────

type selfResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	LastLoginAt any    `json:"last_login_at"`
	CreatedAt   any    `json:"created_at"`
}

func userSelfResponse(u *model.User) selfResponse {
	return selfResponse{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		AvatarURL:   u.AvatarURL,
		Role:        u.Role,
		IsActive:    u.IsActive,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}

func setAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   180, // 3 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{"access_token", "refresh_token"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

func deliverCLIAuthPayload(redirectURI string, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, redirectURI, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return errors.New("cli callback rejected payload")
	}

	return nil
}
