package kagi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

	var requestBody io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
		requestBody = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpointURL(endpointPath, query), requestBody)
	if err != nil {
		return nil, err
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
	resp, err := c.httpClient.Do(req)
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
