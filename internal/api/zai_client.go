// Package api provides clients for interacting with the Z.ai API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Custom errors for Z.ai API failures.
var (
	ErrZaiUnauthorized    = errors.New("zai: unauthorized - invalid API key")
	ErrZaiServerError     = errors.New("zai: server error")
	ErrZaiNetworkError    = errors.New("zai: network error")
	ErrZaiInvalidResponse = errors.New("zai: invalid response")
	ErrZaiAPIError        = errors.New("zai: API returned error")
)

// ZaiClient is an HTTP client for the Z.ai API.
type ZaiClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	logger     *slog.Logger
}

// ZaiOption configures a ZaiClient.
type ZaiOption func(*ZaiClient)

// WithZaiBaseURL sets a custom base URL (for testing).
func WithZaiBaseURL(url string) ZaiOption {
	return func(c *ZaiClient) {
		c.baseURL = url
	}
}

// WithZaiTimeout sets a custom timeout (for testing).
func WithZaiTimeout(timeout time.Duration) ZaiOption {
	return func(c *ZaiClient) {
		c.httpClient.Timeout = timeout
	}
}

// NewZaiClient creates a new Z.ai API client.
func NewZaiClient(apiKey string, logger *slog.Logger, opts ...ZaiOption) *ZaiClient {
	client := &ZaiClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
		apiKey:  apiKey,
		baseURL: "https://api.z.ai/api/monitor/usage/quota/limit",
		logger:  logger,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// zaiFetchAttempts is how many times a poll tries before giving up on the
// cycle.
//
// One transient network failure used to cost a whole polling interval. That was
// tolerable while a deployment had one Z.ai account; with three, all of them
// tick at the same instant, each opens its own TLS connection to the same host,
// and the handshakes contend - three failures inside the same second, observed
// on a live deployment, all of them "TLS handshake timeout".
//
// A lost cycle is not just a gap in a graph: the collector's own health flag is
// derived from how long ago a poll last succeeded, so failing a fetch also makes
// the account look stale.
//
// Two attempts rather than more. This runs every couple of minutes and the next
// cycle is a retry of its own; the point is to survive a blip, not to hammer a
// provider that is genuinely refusing.
const zaiFetchAttempts = 2

// zaiRetryDelay separates the attempts. Long enough that a contended handshake
// is not simply repeated into the same contention, short enough to stay far
// inside the polling interval.
var zaiRetryDelay = 2 * time.Second

// retryDelay is the pause between attempts, never longer than the client's own
// timeout.
//
// A fixed pause is wrong for a client configured to give up in a fraction of a
// second: it would make a fast failure slow, which is the opposite of what a
// short timeout asks for. In production the timeout is thirty seconds and this
// is the constant.
func (c *ZaiClient) retryDelay() time.Duration {
	d := zaiRetryDelay
	if t := c.httpClient.Timeout; t > 0 && t < d {
		return t
	}
	return d
}

// FetchQuotas retrieves the current quota information from the Z.ai API.
//
// A transient network failure is retried once; anything the server actually
// answered - an auth failure, a malformed body - is returned as it is. Retrying
// a refusal would turn one wrong answer into two.
func (c *ZaiClient) FetchQuotas(ctx context.Context) (*ZaiQuotaResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= zaiFetchAttempts; attempt++ {
		resp, err := c.fetchQuotasOnce(ctx)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// Only a network failure is worth repeating, and only if there is time.
		if !errors.Is(err, ErrZaiNetworkError) || attempt == zaiFetchAttempts {
			return nil, err
		}
		c.logger.Debug("retrying Z.ai quota fetch after a network error",
			"attempt", attempt, "error", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.retryDelay()):
		}
	}
	return nil, lastErr
}

func (c *ZaiClient) fetchQuotasOnce(ctx context.Context) (*ZaiQuotaResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("zai: creating request: %w", err)
	}

	// Set headers - Z.ai uses API key directly without Bearer prefix
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("User-Agent", "onwatch/1.0")
	req.Header.Set("Accept", "application/json")

	// Log request (with redacted API key)
	c.logger.Debug("fetching Z.ai quotas",
		"url", c.baseURL,
		"api_key", redactZaiAPIKey(c.apiKey),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrZaiNetworkError, err)
	}
	defer resp.Body.Close()

	// Log response status
	c.logger.Debug("Z.ai quota response received",
		"status", resp.StatusCode,
	)

	// Read response body (bounded to 64KB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("%w: reading body: %v", ErrZaiInvalidResponse, err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty response body", ErrZaiInvalidResponse)
	}

	// Parse the response wrapper first to check for API errors
	var wrapper ZaiResponse[ZaiQuotaResponse]
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrZaiInvalidResponse, err)
	}

	// Handle Z.ai's unique error format: HTTP 200 with error code in body
	if wrapper.Code == 401 {
		return nil, ErrZaiUnauthorized
	}

	if !wrapper.Success {
		return nil, fmt.Errorf("%w: code=%d, msg=%s", ErrZaiAPIError, wrapper.Code, wrapper.Msg)
	}

	// The quota response is already parsed in the wrapper
	quotaResp := wrapper.Data

	// Log usage info if we have limits
	if len(quotaResp.Limits) > 0 {
		timeUsage := float64(0)
		tokensUsage := float64(0)
		for _, limit := range quotaResp.Limits {
			if limit.Type == "TIME_LIMIT" {
				timeUsage = limit.CurrentValue
			} else if limit.Type == "TOKENS_LIMIT" {
				tokensUsage = limit.CurrentValue
			}
		}
		c.logger.Debug("Z.ai quotas fetched successfully",
			"time_usage", timeUsage,
			"tokens_usage", tokensUsage,
		)
	}

	return &quotaResp, nil
}

// redactZaiAPIKey masks the API key for logging.
func redactZaiAPIKey(key string) string {
	if key == "" {
		return "(empty)"
	}

	if len(key) < 8 {
		return "***...***"
	}

	// Show first 4 chars and last 3 chars
	return key[:4] + "***...***" + key[len(key)-3:]
}
