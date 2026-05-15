package kagi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	authorizationScheme = "Bearer"
	// maxErrorBodyBytes caps how much of an error response body we will read
	// into memory, to keep a misbehaving server from exhausting it.
	maxErrorBodyBytes = 64 * 1024
)

func (c *Client) newRequest(ctx context.Context, method, endpointPath string, query url.Values, body any) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("kagi: nil context")
	}

	var (
		bodyBytes   []byte
		requestBody io.Reader
	)
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyBytes = buf
		requestBody = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpointURL(endpointPath, query), requestBody)
	if err != nil {
		return nil, err
	}
	if bodyBytes != nil {
		// Replayable body for retries (and stdlib redirect handling).
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authorizationScheme+" "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}

		resp, err := c.httpClient.Do(req)

		// Context cancellation always wins over retry classification.
		if ctxErr := ctx.Err(); ctxErr != nil {
			if resp != nil {
				drainAndClose(resp.Body)
			}
			return nil, ctxErr
		}

		retryable, delay := c.classifyAttempt(resp, err, attempt)
		if !retryable {
			if err != nil {
				return nil, err
			}
			if resp.StatusCode >= 400 {
				apiErr := parseAPIError(resp)
				drainAndClose(resp.Body)
				return nil, apiErr
			}
			return resp, nil
		}

		// Retry: discard this response (if any) and wait before next attempt.
		if resp != nil {
			drainAndClose(resp.Body)
		}
		if err := c.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

// classifyAttempt returns whether the current attempt should be retried, and
// the delay to wait before retrying. resp is nil iff err is non-nil (transport
// error path).
func (c *Client) classifyAttempt(resp *http.Response, err error, attempt int) (bool, time.Duration) {
	if attempt >= c.maxRetries {
		return false, 0
	}

	if err != nil {
		// Caller-initiated cancellations are not transient.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, 0
		}
		return true, backoffDelay(c.backoffBase, c.backoffMax, attempt)
	}

	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
		return false, 0
	}

	// Honor Retry-After when the server provides it (429/503 typically).
	if ra := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ra > 0 {
		if ra > c.backoffMax {
			ra = c.backoffMax
		}
		return true, ra
	}
	return true, backoffDelay(c.backoffBase, c.backoffMax, attempt)
}

// backoffDelay returns an exponential-backoff window with full jitter for the
// given attempt (0-indexed for the post-failure wait).
func backoffDelay(base, maxBackoff time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	// Saturating left-shift to avoid overflow on long retry sequences.
	window := base
	for i := 0; i < attempt; i++ {
		next := window * 2
		if next <= window || next > maxBackoff {
			window = maxBackoff
			break
		}
		window = next
	}
	if window > maxBackoff {
		window = maxBackoff
	}
	if window <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(window)))
}

// contextSleep waits for d, or until ctx is cancelled. Returns ctx.Err() on
// cancellation and nil on a clean wait.
func contextSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) endpointURL(endpointPath string, query url.Values) string {
	endpoint := c.baseURL.JoinPath(endpointPath)
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String()
}

// parseAPIError builds an APIError from a non-2xx HTTP response. It reads (and
// caps) the body but does not close it; the caller is responsible for closing.
func parseAPIError(resp *http.Response) *APIError {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Kind:       classifyStatus(resp.StatusCode),
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if len(body) > 0 {
		var env errorEnvelope
		if err := json.Unmarshal(body, &env); err == nil {
			apiErr.Details = env.Error
			apiErr.TraceID = env.Meta.Trace
		}
		if apiErr.Details == nil {
			// Keep raw body around when we could not extract structured detail.
			apiErr.Body = body
		}
	}

	if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500 {
		apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	}

	return apiErr
}

func classifyStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	case status >= 500:
		return ErrServerError
	case status >= 400:
		return ErrBadRequest
	}
	return nil
}

// parseRetryAfter parses an HTTP Retry-After header value, which may be either
// a non-negative integer number of seconds or an HTTP-date. Returns zero when
// the header is empty or unparseable.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxErrorBodyBytes))
	_ = body.Close()
}

// errorEnvelope mirrors the API's error response envelope. Only the fields we
// surface on APIError are decoded.
type errorEnvelope struct {
	Meta struct {
		Trace string `json:"trace"`
	} `json:"meta"`
	Error []ErrorDetail `json:"error"`
}
