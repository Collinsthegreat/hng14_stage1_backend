package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type CountryInfo struct {
	CountryID   string  `json:"country_id"`
	Probability float64 `json:"probability"`
}

type NationalizeResponse struct {
	Name      string        `json:"name"`
	Countries []CountryInfo `json:"country"`
}

type NationalizeClient interface {
	Fetch(ctx context.Context, name string) (NationalizeResponse, error)
}

type nationalizeClient struct {
	client  *http.Client
	baseURL string
}

func NewNationalizeClient(client *http.Client, baseURL string) NationalizeClient {
	return &nationalizeClient{
		client:  client,
		baseURL: baseURL,
	}
}

func (c *nationalizeClient) Fetch(ctx context.Context, name string) (NationalizeResponse, error) {
	url := fmt.Sprintf("%s?name=%s", c.baseURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return NationalizeResponse{}, fmt.Errorf("Nationalize returned an invalid response")
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)
	
	outcome := "success"
	if err != nil {
		outcome = "failure"
		slog.Error("external API call", "api", "Nationalize", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return NationalizeResponse{}, fmt.Errorf("Nationalize returned an invalid response")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		outcome = "bad status code"
		slog.Error("external API call", "api", "Nationalize", "name", name, "duration", duration.String(), "outcome", outcome, "status_code", resp.StatusCode)
		return NationalizeResponse{}, fmt.Errorf("Nationalize returned an invalid response")
	}

	var res NationalizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		outcome = "decode error"
		slog.Error("external API call", "api", "Nationalize", "name", name, "duration", duration.String(), "outcome", outcome, "error", err)
		return NationalizeResponse{}, fmt.Errorf("Nationalize returned an invalid response")
	}

	slog.Info("external API call", "api", "Nationalize", "name", name, "duration", duration.String(), "outcome", outcome)

	if len(res.Countries) == 0 {
		return NationalizeResponse{}, fmt.Errorf("Nationalize returned an invalid response")
	}

	return res, nil
}
