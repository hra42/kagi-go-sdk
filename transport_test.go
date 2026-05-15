package kagi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient builds a client pointed at the given test server with retries
// enabled and a deterministic, instant sleep so test runtime stays trivial.
// The recorded slice receives each backoff duration the loop tried to wait,
// in order.
func newTestClient(t *testing.T, server *httptest.Server, retries int, recorded *[]time.Duration) *Client {
	t.Helper()
	client := NewClient("k",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
		WithRetries(retries),
		WithBackoff(1*time.Millisecond, 5*time.Millisecond),
	)
	client.sleep = func(ctx context.Context, d time.Duration) error {
		if recorded != nil {
			*recorded = append(*recorded, d)
		}
		return ctx.Err()
	}
	return client
}

// doPost issues a POST through the test client and returns the resulting
// response or error. The caller is responsible for closing the body on success.
func doPost(t *testing.T, client *Client, body any) (*http.Response, error) {
	t.Helper()
	req, err := client.newRequest(context.Background(), http.MethodPost, "/echo", nil, body)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	return client.do(req)
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := newTestClient(t, server, 3, nil)
	resp, err := doPost(t, client, map[string]string{"q": "hi"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestRetryOn500ThenSuccess(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server, 3, nil)
	resp, err := doPost(t, client, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}

func TestRetryHonorsRetryAfter(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var recorded []time.Duration
	// Set backoff max above the Retry-After to confirm it's the source.
	client := NewClient("k",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
		WithRetries(3),
		WithBackoff(1*time.Millisecond, 10*time.Second),
	)
	client.sleep = func(ctx context.Context, d time.Duration) error {
		recorded = append(recorded, d)
		return nil
	}

	resp, err := doPost(t, client, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if len(recorded) != 1 {
		t.Fatalf("recorded sleeps = %v, want 1", recorded)
	}
	if recorded[0] != 2*time.Second {
		t.Fatalf("backoff delay = %s, want 2s", recorded[0])
	}
}

func TestRetryAfterCappedByBackoffMax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	var recorded []time.Duration
	client := NewClient("k",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
		WithRetries(1),
		WithBackoff(1*time.Millisecond, 250*time.Millisecond),
	)
	client.sleep = func(ctx context.Context, d time.Duration) error {
		recorded = append(recorded, d)
		return nil
	}

	_, err := doPost(t, client, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if len(recorded) != 1 || recorded[0] != 250*time.Millisecond {
		t.Fatalf("recorded = %v, want one 250ms wait", recorded)
	}
}

func TestExhaustsRetriesAndReturnsAPIError(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestClient(t, server, 2, nil)
	_, err := doPost(t, client, nil)
	if !errors.Is(err, ErrServerError) {
		t.Fatalf("err = %v, want ErrServerError", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*APIError) failed: %v", err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", apiErr.StatusCode)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 (initial + 2 retries)", got)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"400", http.StatusBadRequest, ErrBadRequest},
		{"401", http.StatusUnauthorized, ErrUnauthorized},
		{"403", http.StatusForbidden, ErrBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			client := newTestClient(t, server, 3, nil)
			_, err := doPost(t, client, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if got := hits.Load(); got != 1 {
				t.Fatalf("hits = %d, want 1 (no retry)", got)
			}
		})
	}
}

func TestNoRetryWhenDisabled(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server, 0, nil)
	_, err := doPost(t, client, nil)
	if !errors.Is(err, ErrServerError) {
		t.Fatalf("err = %v, want ErrServerError", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

func TestRetryReplaysRequestBody(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server, 3, nil)
	resp, err := doPost(t, client, map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if len(bodies) != 2 {
		t.Fatalf("bodies recorded = %d, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("bodies differ between attempts:\n  first:  %s\n  retry:  %s", bodies[0], bodies[1])
	}
	if !strings.Contains(bodies[1], `"hello":"world"`) {
		t.Fatalf("retry body missing payload: %s", bodies[1])
	}
}

func TestRetryStopsOnContextCancel(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient("k",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
		WithRetries(5),
		WithBackoff(1*time.Millisecond, 5*time.Millisecond),
	)
	// Cancel during the first backoff sleep.
	client.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	req, err := client.newRequest(ctx, http.MethodPost, "/echo", nil, nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	_, err = client.do(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1 (no retry after cancel)", got)
	}
}

func TestRetryOnTransportError(t *testing.T) {
	// Server that closes the connection without responding triggers a
	// transport-level error on every attempt.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter does not support hijack")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		conn.Close()
	}))
	defer server.Close()

	var sleeps int
	client := NewClient("k",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
		WithRetries(2),
		WithBackoff(1*time.Millisecond, 5*time.Millisecond),
	)
	client.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps++
		return nil
	}

	_, err := doPost(t, client, nil)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if sleeps != 2 {
		t.Fatalf("sleeps = %d, want 2 (one per retry)", sleeps)
	}
}

func TestBackoffDelayBoundedByMax(t *testing.T) {
	const base = 100 * time.Millisecond
	const maxBackoff = 1 * time.Second
	for attempt := 0; attempt < 20; attempt++ {
		got := backoffDelay(base, maxBackoff, attempt)
		if got < 0 || got >= maxBackoff {
			t.Errorf("attempt %d: delay = %s, want [0, %s)", attempt, got, maxBackoff)
		}
	}
}

func TestBackoffDelayZeroBase(t *testing.T) {
	if got := backoffDelay(0, time.Second, 3); got != 0 {
		t.Errorf("zero base: delay = %s, want 0", got)
	}
}
