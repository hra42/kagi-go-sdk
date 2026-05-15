# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project

A handbuilt, idiomatic Go SDK for the Kagi Search API. Goal: ergonomic, production-ready, easy to vendor — covering the full API surface with clean types, retry logic, rate limit handling, and proper context propagation.

## Hard constraint: zero external dependencies

The SDK relies **exclusively on the Go standard library**. `go.mod` must have **no `require` entries** beyond the Go version itself. This applies to production code AND tests — no `testify`, `gomock`, or any third-party library. Use stdlib `testing` + `net/http/httptest`. Table-driven tests preferred.

If a task seems to call for a dependency, surface this constraint to the user before adding one.

## Architecture (target)

The SDK is being built up across four milestones; respect this layering when adding code:

- **M1 — Foundation**: base `Client` with functional options pattern (`NewClient(apiKey string, opts ...Option) *Client`; options include `WithTimeout`, `WithHTTPClient`, `WithBaseURL`, `WithRetries`, `WithUserAgent`). Typed error hierarchy (`ErrUnauthorized`, `ErrRateLimited`, `ErrBadRequest`, `ErrServerError`) with `errors.As`-compatible unwrapping. Keep transport layer cleanly separated from API layer.
- **M2 — API Coverage**: `client.Search(ctx, SearchRequest) (*SearchResult, error)` and `client.Extract(ctx, ExtractRequest) (*ExtractResult, error)`. Typed request/response structs — no pointer soup, no `map[string]any`. Full godoc on every exported type.
- **M3 — Production Ready**: retry with exponential backoff + jitter on 429/5xx, respecting `Retry-After`. Expose `RateLimit`, `RateLimitRemaining`, `RateLimitReset` on responses. Retries must be transparent to callers. Tests use `httptest` mock servers covering 401/429/500 paths and retry/backoff behavior.
- **M4 — Launch**: README, godoc, runnable `_examples/` directory.

`ctx context.Context` is the first parameter on every public call that does I/O.

## Commands

Standard Go toolchain only:

- Build: `go build ./...`
- Test all: `go test ./...`
- Test one package verbosely: `go test -v ./path/to/pkg`
- Test one function: `go test -run TestName ./path/to/pkg`
- Race + coverage: `go test -race -cover ./...`
- Lint/vet: `go vet ./...` and `gofmt -s -w .`

CI is intended to be plain `go test ./...` via GitHub Actions — keep it that way.
