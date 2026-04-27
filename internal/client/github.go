package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GitHubUser holds the user fields returned by the GitHub API.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubClient encapsulates GitHub OAuth token exchange and user info retrieval.
type GitHubClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (string, error)
	GetUser(ctx context.Context, accessToken string) (GitHubUser, error)
}

type githubClient struct {
	http         *http.Client
	clientID     string
	clientSecret string
}

func NewGitHubClient(httpClient *http.Client, clientID, clientSecret string) GitHubClient {
	return &githubClient{
		http:         httpClient,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// ExchangeCode exchanges an OAuth authorization code (+ optional PKCE verifier) for an access token.
func (c *githubClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("code", code)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: token exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github: token exchange failed (%d): %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("github: decode token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github: %s — %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("github: empty access token in response")
	}
	return result.AccessToken, nil
}

// GetUser fetches the authenticated user's profile from the GitHub API.
func (c *githubClient) GetUser(ctx context.Context, accessToken string) (GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return GitHubUser{}, fmt.Errorf("github: build user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return GitHubUser{}, fmt.Errorf("github: user request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return GitHubUser{}, fmt.Errorf("github: user fetch failed (%d): %s", resp.StatusCode, body)
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return GitHubUser{}, fmt.Errorf("github: decode user response: %w", err)
	}
	return user, nil
}
