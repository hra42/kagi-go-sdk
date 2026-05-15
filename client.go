// Package kagi is a handbuilt, idiomatic Go SDK for the Kagi Search API.
//
// Construct a [Client] with [NewClient] and call [Client.Search] or
// [Client.Extract]. Transient failures (HTTP 429, 5xx, and network errors)
// are retried transparently with exponential backoff plus full jitter;
// see [WithRetries] and [WithBackoff] to tune the behavior. Errors are
// returned as [*APIError] values wrapping sentinel errors such as
// [ErrUnauthorized] and [ErrRateLimited] for use with [errors.Is] and
// [errors.As].
package kagi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL     = "https://kagi.com/api/v1"
	defaultTimeout     = 30 * time.Second
	defaultUserAgent   = "kagi-go-sdk"
	defaultMaxRetries  = 3
	defaultBackoffBase = 500 * time.Millisecond
	defaultBackoffMax  = 30 * time.Second
)

// Client is a Kagi API client.
//
// Clients are safe to reuse across requests after construction. Requests that
// fail with HTTP 429, 5xx, or transient network errors are retried
// transparently with exponential backoff plus full jitter; see [WithRetries]
// and [WithBackoff] to tune the behavior.
type Client struct {
	apiKey      string
	baseURL     *url.URL
	httpClient  *http.Client
	maxRetries  int
	backoffBase time.Duration
	backoffMax  time.Duration
	userAgent   string
	// sleep is the backoff delay function. Tests swap this; production uses
	// the contextSleep helper which honors ctx cancellation.
	sleep func(ctx context.Context, d time.Duration) error
}

type config struct {
	baseURL     string
	httpClient  *http.Client
	maxRetries  *int
	backoffBase time.Duration
	backoffMax  time.Duration
	timeout     *time.Duration
	userAgent   string
}

// Option configures a Client.
type Option func(*config)

// NewClient creates a new Kagi API client using apiKey for authentication.
func NewClient(apiKey string, opts ...Option) *Client {
	cfg := config{
		baseURL:   defaultBaseURL,
		userAgent: defaultUserAgent,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	baseURL := parseBaseURL(cfg.baseURL)
	httpClient := cloneHTTPClient(cfg.httpClient)
	switch {
	case cfg.timeout != nil:
		httpClient.Timeout = *cfg.timeout
	case cfg.httpClient == nil:
		httpClient.Timeout = defaultTimeout
	}

	maxRetries := defaultMaxRetries
	if cfg.maxRetries != nil {
		maxRetries = *cfg.maxRetries
	}
	backoffBase := cfg.backoffBase
	if backoffBase <= 0 {
		backoffBase = defaultBackoffBase
	}
	backoffMax := cfg.backoffMax
	if backoffMax <= 0 {
		backoffMax = defaultBackoffMax
	}

	return &Client{
		apiKey:      apiKey,
		baseURL:     baseURL,
		httpClient:  httpClient,
		maxRetries:  maxRetries,
		backoffBase: backoffBase,
		backoffMax:  backoffMax,
		userAgent:   cfg.userAgent,
		sleep:       contextSleep,
	}
}

// WithTimeout configures the timeout used by the client's HTTP client.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *config) {
		if timeout <= 0 {
			return
		}
		cfg.timeout = &timeout
	}
}

// WithHTTPClient configures the HTTP client used to make API requests.
//
// The provided client is copied during NewClient construction so later SDK
// options do not mutate caller-owned state.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(cfg *config) {
		if httpClient == nil {
			return
		}
		cfg.httpClient = httpClient
	}
}

// WithBaseURL configures the base URL used for API requests.
//
// The value must be an absolute URL (with scheme and host); invalid or empty
// values are ignored and the previously configured base URL is retained.
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			return
		}
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return
		}
		cfg.baseURL = baseURL
	}
}

// WithRetries configures the maximum number of retries for retryable requests
// (HTTP 429, 5xx, and transient network errors). The default is 3. Pass 0 to
// disable retries; negative values are treated as 0.
//
// Retries are transparent to the caller: a successful retry returns the
// success response, and an exhausted retry budget returns the final error.
func WithRetries(maxRetries int) Option {
	return func(cfg *config) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		cfg.maxRetries = &maxRetries
	}
}

// WithBackoff configures retry backoff timing.
//
// base is the initial backoff window; the window doubles each attempt up to
// maxBackoff. Each delay is jittered uniformly in [0, window). Non-positive
// values leave the corresponding default in place (500ms base, 30s max).
//
// The configured maxBackoff is also the ceiling applied to a server-provided
// Retry-After value.
func WithBackoff(base, maxBackoff time.Duration) Option {
	return func(cfg *config) {
		if base > 0 {
			cfg.backoffBase = base
		}
		if maxBackoff > 0 {
			cfg.backoffMax = maxBackoff
		}
	}
}

// WithUserAgent configures the User-Agent header sent with API requests.
func WithUserAgent(userAgent string) Option {
	return func(cfg *config) {
		userAgent = strings.TrimSpace(userAgent)
		if userAgent == "" {
			return
		}
		cfg.userAgent = userAgent
	}
}

func cloneHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		return &http.Client{}
	}
	clone := *httpClient
	return &clone
}

func parseBaseURL(raw string) *url.URL {
	baseURL, err := url.Parse(raw)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		baseURL, _ = url.Parse(defaultBaseURL)
	}
	return baseURL
}
