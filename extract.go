package kagi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ExtractFormat selects the response serialization format used by the
// Extract endpoint.
type ExtractFormat string

// Supported [ExtractFormat] values.
const (
	// ExtractFormatJSON requests a JSON response envelope (the default).
	ExtractFormatJSON ExtractFormat = "json"
	// ExtractFormatMarkdown requests a raw text/markdown response.
	//
	// The SDK's [Client.Extract] decoder expects JSON; setting this value
	// will cause the response decode to fail. Leave [ExtractRequest.Format]
	// unset to inherit the JSON default.
	ExtractFormatMarkdown ExtractFormat = "markdown"
)

// ExtractPage identifies a single page to extract content from.
type ExtractPage struct {
	// URL is the HTTPS URL of the page to extract. Required.
	URL string `json:"url"`
}

// ExtractRequest is the input to [Client.Extract].
//
// [ExtractRequest.Pages] is required and must contain between 1 and 10
// entries (server-enforced). All other fields are optional and are omitted
// from the request body when left at their zero value.
type ExtractRequest struct {
	// Pages is the set of pages to extract. Server-side limits: 1..10
	// entries, each URL must use the HTTPS scheme.
	Pages []ExtractPage `json:"pages"`

	// Timeout is a time budget for the entire bulk extraction operation,
	// in seconds. Server-side range is 0.5..10; out-of-range values are
	// clamped by the server.
	Timeout float64 `json:"timeout,omitempty"`

	// Format selects the response serialization. Defaults to
	// [ExtractFormatJSON] server-side when unset; see
	// [ExtractFormatMarkdown] for caveats.
	Format ExtractFormat `json:"format,omitempty"`
}

// PageResult is the extracted content for a single page in an
// [ExtractResult].
type PageResult struct {
	// URL is the URL of the extracted page, echoed from the request.
	URL string `json:"url"`
	// Markdown is the extracted page content rendered as markdown. May be
	// empty when extraction yielded no content for this page; check
	// [ExtractResult.Errors] for a matching error entry in that case.
	Markdown string `json:"markdown,omitempty"`
}

// ExtractMeta is the meta envelope of an extract response.
//
// The exact set of fields is intended for debugging and may evolve over time;
// callers should treat it as advisory and avoid building hard dependencies
// on individual fields.
type ExtractMeta struct {
	// Trace is the request trace ID, useful when contacting Kagi support.
	Trace string `json:"trace"`
	// Node identifies the server node that fulfilled the request.
	Node string `json:"node"`
	// MS is the server-side processing time in milliseconds, excluding
	// network round-trip.
	MS int `json:"ms"`
}

// ExtractResult is the response returned by [Client.Extract].
//
// A 200 response may include both extracted [PageResult] entries in
// [ExtractResult.Data] and per-URL failures in [ExtractResult.Errors]; the
// SDK does not convert that partial-success state into an error return.
type ExtractResult struct {
	// Meta carries trace and timing information about the request.
	Meta ExtractMeta
	// Data carries the extracted page results, in the same order as the
	// request's [ExtractRequest.Pages] when the server produced output for
	// each.
	Data []PageResult
	// Errors carries per-URL failures encountered during extraction. Empty
	// when every page extracted cleanly.
	Errors []ErrorDetail
}

// Extract fetches markdown content for one or more pages.
//
// [ExtractRequest.Pages] is required; an empty slice or any entry with a
// blank URL is rejected without contacting the server. Server-side limits
// (1..10 pages, HTTPS scheme, timeout range) are enforced by the API and
// surface as a [*APIError] wrapping [ErrBadRequest].
//
// API errors are returned as a [*APIError] wrapping one of the package's
// sentinel errors (for example, [ErrUnauthorized] or [ErrRateLimited]); use
// [errors.Is] and [errors.As] to classify and inspect them.
func (c *Client) Extract(ctx context.Context, req ExtractRequest) (*ExtractResult, error) {
	if len(req.Pages) == 0 {
		return nil, errors.New("kagi: ExtractRequest.Pages must contain at least one page")
	}
	for i, p := range req.Pages {
		if strings.TrimSpace(p.URL) == "" {
			return nil, fmt.Errorf("kagi: ExtractRequest.Pages[%d].URL is required", i)
		}
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/extract", nil, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	var envelope struct {
		Meta   ExtractMeta   `json:"meta"`
		Data   []PageResult  `json:"data"`
		Errors []ErrorDetail `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("kagi: decode extract response: %w", err)
	}

	return &ExtractResult{Meta: envelope.Meta, Data: envelope.Data, Errors: envelope.Errors}, nil
}
