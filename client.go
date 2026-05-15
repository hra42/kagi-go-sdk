package kagi

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://kagi.com/api/v0"
	defaultTimeout   = 30 * time.Second
	defaultUserAgent = "kagi-go-sdk"
)

// Client is a Kagi API client.
//
// Clients are safe to reuse across requests after construction.
type Client struct {
	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
	maxRetries int
	userAgent  string
}

type config struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	timeout    time.Duration
	timeoutSet bool
	userAgent  string
}

// Option configures a Client.
type Option func(*config)

// NewClient creates a new Kagi API client using apiKey for authentication.
func NewClient(apiKey string, opts ...Option) *Client {
	cfg := config{
		baseURL:   defaultBaseURL,
		timeout:   defaultTimeout,
		userAgent: defaultUserAgent,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	baseURL := parseBaseURL(cfg.baseURL)
	httpClient := cloneHTTPClient(cfg.httpClient)
	if cfg.httpClient == nil || cfg.timeoutSet {
		httpClient.Timeout = cfg.timeout
	}

	return &Client{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: cfg.maxRetries,
		userAgent:  cfg.userAgent,
	}
}

// WithTimeout configures the timeout used by the client's HTTP client.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *config) {
		if timeout <= 0 {
			return
		}
		cfg.timeout = timeout
		cfg.timeoutSet = true
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
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			return
		}
		cfg.baseURL = baseURL
	}
}

// WithRetries configures the maximum number of retries for retryable requests.
//
// Retry execution is implemented by a later production-readiness milestone;
// this option stores the configured value for that layer.
func WithRetries(maxRetries int) Option {
	return func(cfg *config) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		cfg.maxRetries = maxRetries
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
