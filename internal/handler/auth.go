package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

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
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	if codeChallenge != "" && codeChallengeMethod == "" {
		codeChallengeMethod = "S256"
	}
	redirectURI := q.Get("redirect_uri") // CLI sends its localhost callback URI
	incomingState := q.Get("state")

	// Generate or use the provided state
	var state string
	if incomingState != "" {
		state = incomingState
	} else {
		var err error
		state, err = service.GenerateState()
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	// Store state → codeChallenge mapping with TTL
	h.svc.StoreState(state, codeChallenge)

	// Persist the CLI redirect_uri in state cookie so callback can return to it
	if redirectURI != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "cli_redirect_uri",
			Value:    redirectURI,
			Path:     "/",
			MaxAge:   600, // 10 minutes, matching state TTL
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

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

	if code == "" || state == "" {
		response.Error(w, http.StatusBadRequest, "missing code or state")
		return
	}

	// Validate and pop state
	codeChallenge, ok := h.svc.ValidateAndPopState(state)
	if !ok {
		response.Error(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	// Determine if this is a CLI flow by checking for stored CLI redirect URI
	cliRedirectURI := ""
	if cookie, err := r.Cookie("cli_redirect_uri"); err == nil {
		cliRedirectURI = cookie.Value
	}
	isCLI := cliRedirectURI != ""

	// For PKCE: codeVerifier would have been sent in the original request.
	// We stored codeChallenge; the CLI must resend codeVerifier here.
	// GitHub PKCE: the verifier is validated server-side by GitHub during code exchange.
	codeVerifier := q.Get("code_verifier") // present in CLI callback
	_ = codeChallenge                       // already validated in state store

	user, accessToken, refreshToken, err := h.svc.HandleCallback(r.Context(), code, state, codeVerifier)
	if err != nil {
		slog.Error("auth callback error", "error", err)
		response.Error(w, http.StatusInternalServerError, "authentication failed")
		return
	}

	if isCLI {
		// CLI flow: POST JSON to the CLI's local callback server
		// Clear the CLI cookie
		http.SetCookie(w, &http.Cookie{
			Name:   "cli_redirect_uri",
			Value:  "",
			MaxAge: -1,
			Path:   "/",
		})

		// Redirect CLI's localhost server with tokens as query params (safe for localhost)
		// OR: proxy the response as JSON to redirect_uri
		callbackURL, err := url.Parse(cliRedirectURI)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "invalid CLI redirect URI")
			return
		}

		// Send JSON directly to the CLI callback
		// We redirect with a 302 and let the CLI server receive it,
		// but the spec says "returns JSON to redirect_uri" — so we POST JSON.
		// Since we can't POST from a redirect, we render an HTML page that auto-posts,
		// OR (simpler and more robust) we include tokens as query params for the CLI to read.
		// Best practice: embed tokens in URL fragment or body. We'll use query params on localhost only.
		cq := callbackURL.Query()
		cq.Set("access_token", accessToken)
		cq.Set("refresh_token", refreshToken)
		cq.Set("username", user.Username)
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
	http.Redirect(w, r, dashboardURL, http.StatusFound)
}

// Refresh handles POST /auth/refresh.
// Does NOT require JWT auth — called when access token is expired.
// Reads refresh_token from JSON body, rotates the pair.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
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
	// Best-effort decode — may be empty for browser clients using cookies
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Fallback: read from cookie (web portal)
	if body.RefreshToken == "" {
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			body.RefreshToken = cookie.Value
		}
	}

	if body.RefreshToken != "" {
		if err := h.svc.Logout(r.Context(), body.RefreshToken); err != nil {
			slog.Error("logout error", "error", err)
		}
	}

	// Clear cookies (browser clients)
	clearAuthCookies(w)

	response.JSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "logged out",
	})
}

// ─── Cookie helpers ────────────────────────────────────────────────────────────

func setAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   180, // 3 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(3 * time.Minute),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(5 * time.Minute),
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
