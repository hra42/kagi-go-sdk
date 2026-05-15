package kagi

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned for non-2xx API responses. Use [errors.Is] to test
// for a specific category, and [errors.As] with [*APIError] to access the full
// response detail.
//
//	if errors.Is(err, kagi.ErrRateLimited) {
//	    var apiErr *kagi.APIError
//	    if errors.As(err, &apiErr) {
//	        time.Sleep(apiErr.RetryAfter)
//	    }
//	}
var (
	// ErrUnauthorized is returned for HTTP 401 responses, typically caused by a
	// missing or invalid API key.
	ErrUnauthorized = errors.New("kagi: unauthorized")

	// ErrRateLimited is returned for HTTP 429 responses. The accompanying
	// [APIError.RetryAfter] is populated when the server provides a
	// Retry-After header.
	ErrRateLimited = errors.New("kagi: rate limited")

	// ErrBadRequest is returned for HTTP 400 responses (and other non-401
	// client-side 4xx codes), indicating the request was malformed or rejected
	// by validation.
	ErrBadRequest = errors.New("kagi: bad request")

	// ErrServerError is returned for HTTP 5xx responses, which are treated as
	// transient by the retry layer.
	ErrServerError = errors.New("kagi: server error")
)

// ErrorDetail mirrors a single entry of the Kagi API's errorDetail schema.
type ErrorDetail struct {
	// Code is a namespaced error code (for example, "extract.invalid_url").
	Code string `json:"code"`
	// URL points to documentation for the error code.
	URL string `json:"url"`
	// Message is a human-readable description of the error.
	Message string `json:"message,omitempty"`
	// Location identifies the request field where the error occurred, when
	// applicable (for example, "pages[0].url").
	Location string `json:"location,omitempty"`
}

// APIError is the concrete error type returned for non-2xx Kagi API responses.
// It implements [error] and unwraps to one of the sentinel errors
// ([ErrUnauthorized], [ErrRateLimited], [ErrBadRequest], [ErrServerError]) so
// callers can match with [errors.Is].
type APIError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Status is the HTTP status text of the response (for example, "429 Too Many Requests").
	Status string
	// Kind is the sentinel error this APIError unwraps to.
	Kind error
	// Details holds the parsed errorDetail entries from the response envelope.
	// May be empty when the response body was missing, malformed, or did not
	// follow the expected envelope shape.
	Details []ErrorDetail
	// RetryAfter is the parsed Retry-After header duration. It is populated
	// for 429 and 5xx responses when the header is present, and is zero
	// otherwise.
	RetryAfter time.Duration
	// TraceID is the meta.trace value from the response envelope, useful when
	// contacting Kagi support. Empty when not provided.
	TraceID string
	// Body is the raw response body (capped) preserved for debugging when the
	// envelope could not be parsed.
	Body []byte
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := e.firstMessage()
	if msg == "" {
		if e.Status != "" {
			msg = e.Status
		} else {
			msg = "request failed"
		}
	}
	return fmt.Sprintf("kagi: %d: %s", e.StatusCode, msg)
}

// Unwrap returns the sentinel error this APIError wraps, enabling
// [errors.Is] matching against the package's exported sentinels.
func (e *APIError) Unwrap() error { return e.Kind }

func (e *APIError) firstMessage() string {
	for _, d := range e.Details {
		if d.Message != "" {
			return d.Message
		}
	}
	for _, d := range e.Details {
		if d.Code != "" {
			return d.Code
		}
	}
	return ""
}
