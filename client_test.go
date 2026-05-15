package kagi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	client := NewClient("test-key")

	if client.apiKey != "test-key" {
		t.Fatalf("api key = %q, want %q", client.apiKey, "test-key")
	}
	if client.baseURL.String() != defaultBaseURL {
		t.Fatalf("base URL = %q, want %q", client.baseURL.String(), defaultBaseURL)
	}
	if client.httpClient == nil {
		t.Fatal("HTTP client is nil")
	}
	if client.httpClient.Timeout != defaultTimeout {
		t.Fatalf("timeout = %s, want %s", client.httpClient.Timeout, defaultTimeout)
	}
	if client.maxRetries != 0 {
		t.Fatalf("max retries = %d, want 0", client.maxRetries)
	}
	if client.userAgent != defaultUserAgent {
		t.Fatalf("user agent = %q, want %q", client.userAgent, defaultUserAgent)
	}
}

func TestOptionsOverrideConfiguration(t *testing.T) {
	client := NewClient("test-key",
		WithBaseURL("https://example.com/kagi"),
		WithRetries(3),
		WithTimeout(5*time.Second),
		WithUserAgent("my-app/1.0"),
	)

	if client.baseURL.String() != "https://example.com/kagi" {
		t.Fatalf("base URL = %q, want %q", client.baseURL.String(), "https://example.com/kagi")
	}
	if client.maxRetries != 3 {
		t.Fatalf("max retries = %d, want 3", client.maxRetries)
	}
	if client.httpClient.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want %s", client.httpClient.Timeout, 5*time.Second)
	}
	if client.userAgent != "my-app/1.0" {
		t.Fatalf("user agent = %q, want %q", client.userAgent, "my-app/1.0")
	}
}

func TestWithHTTPClientCopiesCallerClient(t *testing.T) {
	callerClient := &http.Client{Timeout: 2 * time.Second}

	client := NewClient("test-key",
		WithHTTPClient(callerClient),
		WithTimeout(4*time.Second),
	)

	if client.httpClient == callerClient {
		t.Fatal("client stored caller-owned HTTP client pointer")
	}
	if callerClient.Timeout != 2*time.Second {
		t.Fatalf("caller client timeout = %s, want %s", callerClient.Timeout, 2*time.Second)
	}
	if client.httpClient.Timeout != 4*time.Second {
		t.Fatalf("client timeout = %s, want %s", client.httpClient.Timeout, 4*time.Second)
	}
}

func TestWithHTTPClientPreservesCallerTimeoutWhenTimeoutNotSet(t *testing.T) {
	callerClient := &http.Client{Timeout: 2 * time.Second}

	client := NewClient("test-key", WithHTTPClient(callerClient))

	if client.httpClient.Timeout != 2*time.Second {
		t.Fatalf("client timeout = %s, want %s", client.httpClient.Timeout, 2*time.Second)
	}
}

func TestIgnoredOptions(t *testing.T) {
	client := NewClient("test-key",
		nil,
		WithBaseURL("   "),
		WithHTTPClient(nil),
		WithRetries(-1),
		WithTimeout(0),
		WithUserAgent("   "),
	)

	if client.baseURL.String() != defaultBaseURL {
		t.Fatalf("base URL = %q, want %q", client.baseURL.String(), defaultBaseURL)
	}
	if client.httpClient.Timeout != defaultTimeout {
		t.Fatalf("timeout = %s, want %s", client.httpClient.Timeout, defaultTimeout)
	}
	if client.maxRetries != 0 {
		t.Fatalf("max retries = %d, want 0", client.maxRetries)
	}
	if client.userAgent != defaultUserAgent {
		t.Fatalf("user agent = %q, want %q", client.userAgent, defaultUserAgent)
	}
}

func TestInvalidBaseURLFallsBackToDefault(t *testing.T) {
	client := NewClient("test-key", WithBaseURL("://not-a-url"))

	if client.baseURL.String() != defaultBaseURL {
		t.Fatalf("base URL = %q, want %q", client.baseURL.String(), defaultBaseURL)
	}
}

func TestNewRequestBuildsURLAndHeaders(t *testing.T) {
	client := NewClient("test-key",
		WithBaseURL("https://example.com/api/v0/"),
		WithUserAgent("my-app/1.0"),
	)

	req, err := client.newRequest(context.Background(), http.MethodGet, "/search", url.Values{
		"q": []string{"hello world"},
	}, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if req.URL.String() != "https://example.com/api/v0/search?q=hello+world" {
		t.Fatalf("request URL = %q, want %q", req.URL.String(), "https://example.com/api/v0/search?q=hello+world")
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want %q", got, "application/json")
	}
	if got := req.Header.Get("Authorization"); got != "Bot test-key" {
		t.Fatalf("Authorization = %q, want %q", got, "Bot test-key")
	}
	if got := req.Header.Get("User-Agent"); got != "my-app/1.0" {
		t.Fatalf("User-Agent = %q, want %q", got, "my-app/1.0")
	}
	if got := req.Header.Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func TestDoUsesConfiguredHTTPClient(t *testing.T) {
	type requestBody struct {
		Query string `json:"query"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/api/v0/search" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v0/search")
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit query = %q, want %q", r.URL.Query().Get("limit"), "10")
		}
		if got := r.Header.Get("Authorization"); got != "Bot test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bot test-key")
		}
		if got := r.Header.Get("User-Agent"); got != "my-app/1.0" {
			t.Errorf("User-Agent = %q, want %q", got, "my-app/1.0")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if body.Query != "hello world" {
			t.Errorf("body query = %q, want %q", body.Query, "hello world")
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/api/v0"),
		WithHTTPClient(server.Client()),
		WithUserAgent("my-app/1.0"),
	)

	req, err := client.newRequest(context.Background(), http.MethodPost, "search", url.Values{
		"limit": []string{"10"},
	}, requestBody{Query: "hello world"})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
}

func TestNewRequestRejectsNilContext(t *testing.T) {
	client := NewClient("test-key")

	_, err := client.newRequest(nil, http.MethodGet, "search", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil context")
	}
}
