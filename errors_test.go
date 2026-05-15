package kagi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantSentine error
	}{
		{"401 unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"429 rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"400 bad request", http.StatusBadRequest, ErrBadRequest},
		{"403 forbidden classified as bad request", http.StatusForbidden, ErrBadRequest},
		{"404 not found classified as bad request", http.StatusNotFound, ErrBadRequest},
		{"422 unprocessable classified as bad request", http.StatusUnprocessableEntity, ErrBadRequest},
		{"500 server error", http.StatusInternalServerError, ErrServerError},
		{"503 server error", http.StatusServiceUnavailable, ErrServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			err := doGet(t, server)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantSentine) {
				t.Fatalf("errors.Is mismatch: err=%v, want sentinel=%v", err, tt.wantSentine)
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As to *APIError failed for %v", err)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestAPIErrorParsesEnvelope(t *testing.T) {
	body := `{
        "meta": {"trace": "abc-123", "node": "n1", "ms": 12},
        "data": [],
        "error": [
            {
                "code": "extract.invalid_url",
                "url": "https://help.kagi.com/api/errors#extract.invalid_url",
                "message": "URL must be a valid HTTPS URL",
                "location": "pages[0].url"
            }
        ]
    }`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if apiErr.TraceID != "abc-123" {
		t.Fatalf("TraceID = %q, want %q", apiErr.TraceID, "abc-123")
	}
	if len(apiErr.Details) != 1 {
		t.Fatalf("Details len = %d, want 1", len(apiErr.Details))
	}
	d := apiErr.Details[0]
	if d.Code != "extract.invalid_url" {
		t.Errorf("Code = %q", d.Code)
	}
	if d.Message != "URL must be a valid HTTPS URL" {
		t.Errorf("Message = %q", d.Message)
	}
	if d.Location != "pages[0].url" {
		t.Errorf("Location = %q", d.Location)
	}
	if apiErr.Body != nil {
		t.Errorf("Body should be nil when envelope parsed, got %d bytes", len(apiErr.Body))
	}
	if got := apiErr.Error(); !strings.Contains(got, "URL must be a valid HTTPS URL") {
		t.Errorf("Error() = %q, want to contain detail message", got)
	}
}

func TestAPIErrorRetryAfterSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if apiErr.RetryAfter != 5*time.Second {
		t.Fatalf("RetryAfter = %s, want 5s", apiErr.RetryAfter)
	}
}

func TestAPIErrorRetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	// Allow some clock-drift slack on either side of the 30s target.
	if apiErr.RetryAfter < 15*time.Second || apiErr.RetryAfter > 45*time.Second {
		t.Fatalf("RetryAfter = %s, want ~30s", apiErr.RetryAfter)
	}
}

func TestAPIErrorRetryAfterAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if apiErr.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %s, want 0", apiErr.RetryAfter)
	}
}

func TestAPIErrorRetryAfterOn503(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if !errors.Is(err, ErrServerError) {
		t.Fatalf("expected ErrServerError, got %v", err)
	}
	if apiErr.RetryAfter != 12*time.Second {
		t.Fatalf("RetryAfter = %s, want 12s", apiErr.RetryAfter)
	}
}

func TestAPIErrorMalformedBodyPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "not json {{{")
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if len(apiErr.Details) != 0 {
		t.Fatalf("Details len = %d, want 0", len(apiErr.Details))
	}
	if string(apiErr.Body) != "not json {{{" {
		t.Fatalf("Body = %q, want raw bytes preserved", string(apiErr.Body))
	}
	if !errors.Is(err, ErrServerError) {
		t.Fatalf("classification lost on malformed body: %v", err)
	}
}

func TestAPIErrorBodyCap(t *testing.T) {
	huge := strings.Repeat("x", maxErrorBodyBytes+10_000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, huge)
	}))
	defer server.Close()

	err := doGet(t, server)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if len(apiErr.Body) != maxErrorBodyBytes {
		t.Fatalf("Body length = %d, want capped at %d", len(apiErr.Body), maxErrorBodyBytes)
	}
}

func TestDoLeavesSuccessBodyReadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	req, err := client.newRequest(context.Background(), http.MethodGet, "/", nil, nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	resp, err := client.do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("body = %q", string(got))
	}
}

func TestAPIErrorMessageFallsBackToStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	err := doGet(t, server)
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Fatalf("Error() = %q, want to contain status code", msg)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	cases := []string{"", "  ", "not-a-number", "-3"}
	for _, c := range cases {
		if got := parseRetryAfter(c, time.Now()); got != 0 {
			t.Errorf("parseRetryAfter(%q) = %s, want 0", c, got)
		}
	}
}

// doGet issues a GET against the test server and returns the error from do().
// Retries are disabled so error-shape assertions don't pay backoff costs.
func doGet(t *testing.T, server *httptest.Server) error {
	t.Helper()
	client := NewClient("k", WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithRetries(0))
	req, err := client.newRequest(context.Background(), http.MethodGet, "/", nil, nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	_, err = client.do(req)
	return err
}
