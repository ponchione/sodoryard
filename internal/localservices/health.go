package localservices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type HealthHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func defaultHTTPClient() HealthHTTPClient {
	return &http.Client{Timeout: 3 * time.Second}
}

func ProbeService(ctx context.Context, client HealthHTTPClient, name string, service appconfig.ManagedService) ServiceStatus {
	baseURL := strings.TrimRight(service.BaseURL, "/")
	status := ServiceStatus{
		Name:      name,
		Required:  service.Required,
		BaseURL:   baseURL,
		HealthURL: baseURL + service.HealthPath,
		ModelsURL: baseURL + service.ModelsPath,
	}
	if strings.TrimSpace(baseURL) == "" {
		status.Detail = "base_url is empty"
		return status
	}
	if err := requireHTTP200(ctx, client, status.HealthURL); err != nil {
		status.Detail = err.Error()
		return status
	}
	status.Reachable = true
	if err := requireModelsEndpoint(ctx, client, status.ModelsURL); err != nil {
		status.Detail = err.Error()
		return status
	}
	status.ModelsReady = true
	status.Healthy = true
	return status
}

func requireHTTP200(ctx context.Context, client HealthHTTPClient, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health endpoint not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func requireModelsEndpoint(ctx context.Context, client HealthHTTPClient, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build models request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("models endpoint not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("models endpoint returned HTTP %d", resp.StatusCode)
	}
	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return fmt.Errorf("decode models response: %w", err)
	}
	if len(models.Data) == 0 {
		return fmt.Errorf("models endpoint returned no models")
	}
	return nil
}
