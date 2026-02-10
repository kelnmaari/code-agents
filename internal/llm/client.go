package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Client is an OpenAI-compatible HTTP client for chat completions.
// Thread-safe: can be used from multiple goroutines concurrently.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Verify Client implements Completer.
var _ Completer = (*Client)(nil)

// NewClient creates a new LLM client targeting the given OpenAI-compatible endpoint.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // long timeout for LLM completions
		},
	}
}

// Complete sends a chat completion request and returns the response.
// Retries up to 3 times on HTTP 429 or 5xx with exponential backoff.
func (c *Client) Complete(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	backoff := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
	var lastErr error

	for attempt := 0; attempt <= len(backoff); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff[attempt-1]):
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			// Only retry on retryable errors
			if attempt < len(backoff) && isRetryable(err) {
				log.Printf("[llm] request failed (attempt %d/%d), retrying in %s: %v", attempt+1, len(backoff)+1, backoff[attempt], err)
				continue
			}
			return nil, err
		}
		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) doRequest(ctx context.Context, body []byte) (*ChatResponse, error) {
	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusTooManyRequests {
		return nil, &retryableError{
			statusCode: httpResp.StatusCode,
			body:       truncate(string(respBody), 200),
		}
	}

	if httpResp.StatusCode >= 500 {
		return nil, &retryableError{
			statusCode: httpResp.StatusCode,
			body:       truncate(string(respBody), 200),
		}
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, truncate(string(respBody), 200))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp, nil
}

// retryableError indicates the request can be retried.
type retryableError struct {
	statusCode int
	body       string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable error (status %d): %s", e.statusCode, e.body)
}

func isRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
