package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type GenderizeResponse struct {
	Name        string  `json:"name"`
	Gender      *string `json:"gender"`
	Probability float64 `json:"probability"`
	Count       int     `json:"count"`
}

type GenderizeClient interface {
	Fetch(ctx context.Context, name string) (GenderizeResponse, error)
}

type genderizeClient struct {
	client  *http.Client
	baseURL string
}

func NewGenderizeClient(client *http.Client, baseURL string) GenderizeClient {
	return &genderizeClient{
		client:  client,
		baseURL: baseURL,
	}
}

func (c *genderizeClient) Fetch(ctx context.Context, name string) (GenderizeResponse, error) {
	url := fmt.Sprintf("%s?name=%s", c.baseURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GenderizeResponse{}, fmt.Errorf("Genderize returned an invalid response")
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	outcome := "success"
	if err != nil {
		outcome = "failure"
		slog.Error("external API call", "api", "Genderize", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return GenderizeResponse{}, fmt.Errorf("Genderize returned an invalid response")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		outcome = "bad status code"
		slog.Error("external API call", "api", "Genderize", "name", name, "duration", duration.String(), "outcome", outcome, "status_code", resp.StatusCode)
		return GenderizeResponse{}, fmt.Errorf("Genderize returned an invalid response")
	}

	var res GenderizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		outcome = "decode error"
		slog.Error("external API call", "api", "Genderize", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return GenderizeResponse{}, fmt.Errorf("Genderize returned an invalid response")
	}

	slog.Info("external API call", "api", "Genderize", "name", name, "duration", duration.String(), "outcome", outcome)

	if res.Gender == nil || res.Count == 0 {
		return GenderizeResponse{}, fmt.Errorf("Genderize returned an invalid response")
	}

	return res, nil
}
