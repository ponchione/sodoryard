package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	maxBodyBytes   = 2 * 1024 * 1024 // 2 MB cap on response bodies
)

// SearchHit represents a single result from a keyword search.
type SearchHit struct {
	Path    string  `json:"filename"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// ObsidianClient is an HTTP client for the Obsidian Local REST API plugin.
type ObsidianClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewObsidianClient constructs an ObsidianClient targeting the given base URL
// (e.g. "http://localhost:27124") with the provided API key.
func NewObsidianClient(baseURL string, apiKey string) *ObsidianClient {
	return &ObsidianClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// ReadDocument fetches a vault-relative document by path.
// Returns the raw markdown content.
func (c *ObsidianClient) ReadDocument(ctx context.Context, path string) (string, error) {
	u := c.baseURL + "/vault/" + url.PathEscape(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/markdown")

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return "", err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized:
		return "", fmt.Errorf("Obsidian REST API authentication failed. Check the API key in sirtopham.yaml brain.obsidian_api_key.")
	case http.StatusNotFound:
		return "", fmt.Errorf("Document not found: %s", path)
	default:
		return "", fmt.Errorf("Obsidian REST API error (%d): %s", resp.StatusCode, truncateBody(body, 200))
	}
}

// WriteDocument creates or overwrites a vault-relative document.
func (c *ObsidianClient) WriteDocument(ctx context.Context, path string, content string) error {
	u := c.baseURL + "/vault/" + url.PathEscape(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "text/markdown")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("Obsidian REST API authentication failed. Check the API key in sirtopham.yaml brain.obsidian_api_key.")
	default:
		return fmt.Errorf("Obsidian REST API error (%d): %s", resp.StatusCode, truncateBody(body, 200))
	}
}

// SearchKeyword performs a keyword search against the vault.
func (c *ObsidianClient) SearchKeyword(ctx context.Context, query string) ([]SearchHit, error) {
	u := c.baseURL + "/search/simple/?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// The Obsidian REST API returns an array of objects with "filename" and "score"
		// plus optional "matches" which contain snippets.
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			return nil, fmt.Errorf("parse search response: %w", err)
		}
		hits := make([]SearchHit, 0, len(raw))
		for _, item := range raw {
			var hit searchResultItem
			if err := json.Unmarshal(item, &hit); err != nil {
				continue
			}
			snippet := ""
			if len(hit.Matches) > 0 && len(hit.Matches[0].Matches) > 0 {
				snippet = hit.Matches[0].Matches[0].Context
			}
			hits = append(hits, SearchHit{
				Path:    hit.Filename,
				Snippet: snippet,
				Score:   hit.Score,
			})
		}
		return hits, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("Obsidian REST API authentication failed. Check the API key in sirtopham.yaml brain.obsidian_api_key.")
	default:
		return nil, fmt.Errorf("Obsidian REST API error (%d): %s", resp.StatusCode, truncateBody(body, 200))
	}
}

// ListDocuments lists vault-relative document paths under a directory.
// Pass "" to list from the vault root.
func (c *ObsidianClient) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	u := c.baseURL + "/vault/"
	if directory != "" {
		u += url.PathEscape(directory) + "/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var listing struct {
			Files []string `json:"files"`
		}
		if err := json.Unmarshal([]byte(body), &listing); err != nil {
			return nil, fmt.Errorf("parse listing response: %w", err)
		}
		return listing.Files, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("Obsidian REST API authentication failed. Check the API key in sirtopham.yaml brain.obsidian_api_key.")
	case http.StatusNotFound:
		return nil, fmt.Errorf("Directory not found: %s", directory)
	default:
		return nil, fmt.Errorf("Obsidian REST API error (%d): %s", resp.StatusCode, truncateBody(body, 200))
	}
}

// searchResultItem is the Obsidian REST API's search result shape.
type searchResultItem struct {
	Filename string `json:"filename"`
	Score    float64 `json:"score"`
	Matches  []searchResultContext `json:"matches"`
}

type searchResultContext struct {
	Matches []searchResultMatch `json:"matches"`
}

type searchResultMatch struct {
	Context string `json:"context"`
}

// do executes a request and translates connection errors.
func (c *ObsidianClient) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isConnectionError(err) {
			return nil, fmt.Errorf("Cannot connect to Obsidian REST API at %s. Is Obsidian running with the Local REST API plugin enabled?", c.baseURL)
		}
		return nil, fmt.Errorf("obsidian API request failed: %w", err)
	}
	return resp, nil
}

// isConnectionError checks whether an error is a connection-level failure.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if ok := errorAs(err, &netErr); ok {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial tcp")
}

// errorAs is a thin wrapper so we can reference errors.As behavior inline.
func errorAs(err error, target interface{}) bool {
	switch target.(type) {
	case **net.OpError:
		for e := err; e != nil; {
			if oe, ok := e.(*net.OpError); ok {
				*(target.(**net.OpError)) = oe
				return true
			}
			u, ok := e.(interface{ Unwrap() error })
			if !ok {
				break
			}
			e = u.Unwrap()
		}
	}
	return false
}

func readBody(resp *http.Response) (string, error) {
	limited := io.LimitReader(resp.Body, maxBodyBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	return string(data), nil
}

func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return strings.TrimSpace(body)
	}
	return strings.TrimSpace(body[:maxLen]) + "..."
}
