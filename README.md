# kagi-go-sdk

[![CI](https://github.com/hra42/kagi-go-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/hra42/kagi-go-sdk/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/hra42/kagi-go-sdk.svg)](https://pkg.go.dev/github.com/hra42/kagi-go-sdk)
[![Go Version](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go)](go.mod)
[![License: Unlicense](https://img.shields.io/badge/license-Unlicense-blue.svg)](LICENSE)

A handbuilt, idiomatic Go SDK for the [Kagi Search API](https://help.kagi.com/kagi/api/overview.html).

Designed to be ergonomic, production-ready, and easy to vendor — covering the full API surface with clean types, transparent retries, rate-limit handling, and proper context propagation.

- [Why this SDK](#why-this-sdk)
- [vs. `kagisearch/kagi-openapi-golang`](#vs-kagisearchkagi-openapi-golang)
- [Install](#install)
- [Quick start](#quick-start)
- [API reference](#api-reference)
- [Configuration](#configuration)
- [Error handling](#error-handling)
- [Retries and rate limits](#retries-and-rate-limits)
- [Runnable examples](#runnable-examples)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Why this SDK

- **Zero external dependencies.** Standard library only — `go.mod` has no `require` entries beyond the Go version itself. Safe to vendor, trivial to audit.
- **Idiomatic Go.** Functional options, typed request/response structs, `context.Context` on every call, an `errors.Is` / `errors.As`-compatible error hierarchy.
- **Production-ready.** Transparent retries with exponential backoff plus full jitter, `Retry-After` honoring, parsed error envelopes with trace IDs.
- **Handwritten, not generated.** Clean field names, real godoc on every export, no pointer soup, typed enums instead of bare string constants.

## vs. `kagisearch/kagi-openapi-golang`

Kagi publishes [an official client](https://github.com/kagisearch/kagi-openapi-golang) generated from their OpenAPI spec. It is the right choice if you want lockstep with the spec; pick this SDK when you'd rather have ergonomics and built-in resilience.

| | `kagi-go-sdk` (this) | `kagisearch/kagi-openapi-golang` |
|---|---|---|
| Source | Handwritten | OpenAPI-generated |
| External dependencies | None | Generator runtime + transitive deps |
| Error model | Sentinel hierarchy (`ErrUnauthorized`, `ErrRateLimited`, `ErrBadRequest`, `ErrServerError`) wrapped by `*APIError` | Generic `*GenericOpenAPIError` |
| Enums | Typed (`Workflow`, `TimeRelative`, `DomainRuleKind`) | String constants |
| Retries on 429 / 5xx | Built-in, transparent, jittered backoff, honors `Retry-After` | Not provided — caller's responsibility |
| `context.Context` on every call | Yes | Yes |
| Godoc coverage | Every exported symbol | Generated stubs |
| Vendoring footprint | Single module | Module + generated client tree |

## Install

```sh
go get github.com/hra42/kagi-go-sdk
```

Requires **Go 1.26+**.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	kagi "github.com/hra42/kagi-go-sdk"
)

func main() {
	client := kagi.NewClient(os.Getenv("KAGI_API_KEY"))

	res, err := client.Search(context.Background(), kagi.SearchRequest{
		Query: "context propagation in go",
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, hit := range res.Data.Search {
		fmt.Printf("%s\n  %s\n\n", hit.Title, hit.URL)
	}
}
```

## API reference

The full API surface is two methods on `*Client`. Every method takes a `context.Context` and a typed request struct, returns a typed response struct, and reports errors as `*APIError` values wrapping one of four sentinel errors (see [Error handling](#error-handling)).

### `Client.Search`

```go
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResult, error)
```

`SearchRequest` fields:

| Field | Type | Notes |
|---|---|---|
| `Query` | `string` | **Required.** The search query. |
| `Workflow` | `Workflow` | `WorkflowSearch` (default) / `WorkflowImages` / `WorkflowVideos` / `WorkflowNews` / `WorkflowPodcasts`. |
| `LensID` | `string` | Saved-lens ID or full `https://kagi.com/lenses/...` URL. |
| `Lens` | `*Lens` | Inline lens (site/keyword filters, file type, time window, region). |
| `Filters` | `*SearchFilters` | Coarse filters: `Region`, `After`, `Before`. Take priority over `Lens`. |
| `Extract` | `*SearchExtract` | Opt in to inline page extraction for top results (`Count`, `Timeout`). Billed at the Extract rate. |
| `Personalizations` | `*Personalizations` | Per-request `DomainRule` and `RegexRule` ranking tweaks (up to 1000 each). |
| `Page` | `int` | 1-indexed page, server range 1..10. |
| `Limit` | `int` | Result cap, server range 1..1024. |
| `Timeout` | `float64` | Server-side time budget in seconds (0.5..4). |
| `SafeSearch` | `*bool` | Override server default (`true`). Leave `nil` to inherit. |

`SearchResult.Data` groups hits into category buckets — `Search`, `Image`, `Video`, `News`, `Podcast`, `PodcastCreator`, `AdjacentQuestion`, `DirectAnswer`, `InterestingNews`, `InterestingFinds`, `Infobox`, `Code`, `PackageTracking`, `PublicRecords`, `Weather`, `RelatedSearch`, `Listicle`, `WebArchive`. Each is a `[]SearchHit`; category-specific extras are exposed as raw JSON via `SearchHit.Props`.

A richer call combining a lens, ranking rules, and inline extraction:

```go
safe := false
res, err := client.Search(ctx, kagi.SearchRequest{
	Query: "kagi search api",
	Lens: &kagi.Lens{
		SitesIncluded: []string{"help.kagi.com"},
		TimeRelative:  kagi.TimeRelativeMonth,
	},
	Personalizations: &kagi.Personalizations{
		Domains: []kagi.DomainRule{
			{Domain: "blog.kagi.com", Kind: kagi.DomainRuleRaise},
			{Domain: "reddit.com", Kind: kagi.DomainRuleBlock},
		},
	},
	Extract:    &kagi.SearchExtract{Count: 3, Timeout: 2.0},
	Limit:      10,
	SafeSearch: &safe,
})
```

### `Client.Extract`

```go
func (c *Client) Extract(ctx context.Context, req ExtractRequest) (*ExtractResult, error)
```

`ExtractRequest` fields:

| Field | Type | Notes |
|---|---|---|
| `Pages` | `[]ExtractPage` | **Required.** 1..10 entries; each `URL` must be HTTPS. |
| `Timeout` | `float64` | Bulk time budget in seconds (0.5..10). |

A 200 response can include both successful `Data` entries and per-URL failures in `Errors`; partial success is not converted into a Go error.

```go
res, err := client.Extract(ctx, kagi.ExtractRequest{
	Pages: []kagi.ExtractPage{
		{URL: "https://blog.kagi.com/kagi-search-api"},
		{URL: "https://help.kagi.com/kagi/api/search.html"},
	},
})
if err != nil {
	log.Fatal(err)
}

for _, p := range res.Data {
	fmt.Printf("=== %s ===\n%s\n", p.URL, p.Markdown)
}
for _, e := range res.Errors {
	fmt.Printf("failed %s: %s\n", e.Location, e.Message)
}
```

### Enums

| Type | Values |
|---|---|
| `Workflow` | `WorkflowSearch`, `WorkflowImages`, `WorkflowVideos`, `WorkflowNews`, `WorkflowPodcasts` |
| `TimeRelative` | `TimeRelativeDay`, `TimeRelativeWeek`, `TimeRelativeMonth` |
| `DomainRuleKind` | `DomainRuleBlock`, `DomainRuleLower`, `DomainRuleRaise`, `DomainRulePin` |

## Configuration

The client uses the functional options pattern:

```go
client := kagi.NewClient(apiKey,
	kagi.WithTimeout(30*time.Second),
	kagi.WithRetries(3),
	kagi.WithBackoff(500*time.Millisecond, 30*time.Second),
	kagi.WithUserAgent("my-app/1.0"),
	kagi.WithHTTPClient(myHTTPClient),
	kagi.WithBaseURL("https://kagi.com/api/v1"),
)
```

| Option | Default | Effect |
|---|---|---|
| `WithTimeout(d time.Duration)` | `30s` | Sets `http.Client.Timeout`. Ignored when `d <= 0`. |
| `WithHTTPClient(c *http.Client)` | `&http.Client{Timeout: 30s}` | Replaces the transport. The client is shallow-copied at construction, so later mutation of your `*http.Client` does not affect the SDK. |
| `WithBaseURL(s string)` | `https://kagi.com/api/v1` | Override endpoint. Must be an absolute URL with scheme + host; invalid values are silently ignored. |
| `WithRetries(n int)` | `3` | Max retries on 429 / 5xx / transient network errors. `0` disables retries; negative values clamp to `0`. |
| `WithBackoff(base, max time.Duration)` | `500ms`, `30s` | Exponential backoff bounds; each delay is jittered uniformly in `[0, window)`. `max` is also the ceiling for `Retry-After`. Non-positive values keep the default. |
| `WithUserAgent(s string)` | `kagi-go-sdk` | `User-Agent` header. Whitespace-only values are ignored. |

The `*Client` is safe for concurrent reuse — construct it once and share it.

## Error handling

Non-2xx responses are returned as `*APIError`, which unwraps to one of four sentinel errors:

| Sentinel | Trigger |
|---|---|
| `ErrUnauthorized` | HTTP 401 — missing or invalid API key. |
| `ErrRateLimited` | HTTP 429 — quota exhausted. `APIError.RetryAfter` is set when the server provides a `Retry-After` header. |
| `ErrBadRequest` | HTTP 400 and other client-side 4xx (except 401). |
| `ErrServerError` | HTTP 5xx — treated as transient by the retry layer. |

Use `errors.Is` to classify and `errors.As` to read the response detail:

```go
res, err := client.Search(ctx, req)
if err != nil {
	switch {
	case errors.Is(err, kagi.ErrRateLimited):
		var apiErr *kagi.APIError
		if errors.As(err, &apiErr) {
			log.Printf("rate limited, retry after %s (trace %s)", apiErr.RetryAfter, apiErr.TraceID)
		}
	case errors.Is(err, kagi.ErrUnauthorized):
		log.Fatal("check KAGI_API_KEY")
	case errors.Is(err, kagi.ErrBadRequest):
		var apiErr *kagi.APIError
		if errors.As(err, &apiErr) {
			for _, d := range apiErr.Details {
				log.Printf("  %s @ %s: %s", d.Code, d.Location, d.Message)
			}
		}
	default:
		log.Printf("transport or server error: %v", err)
	}
	return
}
```

Useful fields on `*APIError`:

- `StatusCode int` / `Status string` — raw HTTP status.
- `Kind error` — the sentinel this wraps.
- `Details []ErrorDetail` — parsed entries (`Code`, `URL`, `Message`, `Location`) from the response envelope.
- `RetryAfter time.Duration` — populated for 429 and 5xx when a `Retry-After` header is present.
- `TraceID string` — `meta.trace` from the response envelope; include this when contacting Kagi support.
- `Body []byte` — raw (capped) response body, preserved when the envelope failed to parse.

## Retries and rate limits

Requests that fail with HTTP 429, 5xx, or a transient network error are retried transparently. The retry layer:

- Honors any server-provided `Retry-After` header (delta-seconds or HTTP-date), capped at `WithBackoff`'s `max`.
- Otherwise sleeps with exponential backoff plus full jitter: each attempt's window doubles up to `max`, and the actual delay is uniform in `[0, window)`.
- Aborts immediately if the request context is cancelled.

Only the final failure after the retry budget is exhausted is returned to the caller. Set `WithRetries(0)` to disable.

For manual rate-limit handling (for example, deferring work to a queue), check `apiErr.RetryAfter` once the retry budget has been exhausted.

## Runnable examples

Each example is a self-contained `package main`. Set `KAGI_API_KEY` and run:

```sh
go run ./_examples/search           # minimal search
go run ./_examples/extract          # markdown extraction with per-URL error handling
go run ./_examples/search-advanced  # lens, filters, domain rules, inline extract
go run ./_examples/custom-client    # all Options + custom *http.Client + APIError classification
```

## Contributing

Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a PR. Short version: zero external dependencies — production code **and** tests — and every new exported symbol carries godoc.

## Security

Please do not file public issues for security reports. See [`SECURITY.md`](SECURITY.md) — report privately via GitHub's [security advisory flow](https://github.com/hra42/kagi-go-sdk/security/advisories/new).

## License

Released into the public domain under the [Unlicense](LICENSE).
