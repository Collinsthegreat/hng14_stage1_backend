package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type AgifyResponse struct {
	Name  string `json:"name"`
	Age   *int   `json:"age"`
	Count int    `json:"count"`
}

type AgifyClient interface {
	Fetch(ctx context.Context, name string) (AgifyResponse, error)
}

type agifyClient struct {
	client  *http.Client
	baseURL string
}

func NewAgifyClient(client *http.Client, baseURL string) AgifyClient {
	return &agifyClient{
		client:  client,
		baseURL: baseURL,
	}
}

func (c *agifyClient) Fetch(ctx context.Context, name string) (AgifyResponse, error) {
	url := fmt.Sprintf("%s?name=%s", c.baseURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return AgifyResponse{}, fmt.Errorf("Agify returned an invalid response")
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	outcome := "success"
	if err != nil {
		outcome = "failure"
		slog.Error("external API call", "api", "Agify", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return AgifyResponse{}, fmt.Errorf("Agify returned an invalid response")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		outcome = "bad status code"
		slog.Error("external API call", "api", "Agify", "name", name, "duration", duration.String(), "outcome", outcome, "status_code", resp.StatusCode)
		return AgifyResponse{}, fmt.Errorf("Agify returned an invalid response")
	}

	var res AgifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		outcome = "decode error"
		slog.Error("external API call", "api", "Agify", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return AgifyResponse{}, fmt.Errorf("Agify returned an invalid response")
	}

	slog.Info("external API call", "api", "Agify", "name", name, "duration", duration.String(), "outcome", outcome)

	if res.Age == nil {
		return AgifyResponse{}, fmt.Errorf("Agify returned an invalid response")
	}

	return res, nil
}
