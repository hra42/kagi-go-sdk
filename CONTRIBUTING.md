# Contributing

Thanks for your interest in `kagi-go-sdk`. This project is small and opinionated; please read this guide before opening a PR.

## Hard rule: zero external dependencies

The SDK relies **exclusively on the Go standard library**. `go.mod` must have **no `require` entries** beyond the Go version itself. This applies to production code **and tests** — no `testify`, `gomock`, or any third-party library. Use stdlib `testing` + `net/http/httptest`.

If a task seems to call for a dependency, open an issue first so we can discuss alternatives. PRs that add dependencies will not be merged.

## Development setup

```sh
git clone https://github.com/hra42/kagi-go-sdk
cd kagi-go-sdk
go build ./...
go test -race -cover ./...
```

Requires Go 1.26+ (matching `go.mod`).

To exercise the runnable examples against the real API, set `KAGI_API_KEY`:

```sh
export KAGI_API_KEY=...
go run ./_examples/search
```

## Commands

| Command | Purpose |
|---|---|
| `go build ./...` | Compile every package. |
| `go test ./...` | Run the full test suite. |
| `go test -race -cover ./...` | What CI runs. |
| `go vet ./...` | Static checks. |
| `gofmt -s -w .` | Apply the canonical formatting. |
| `golangci-lint run` | Optional — CI runs golangci-lint v2.12. |

## Test conventions

- **Table-driven tests** are preferred for anything with multiple cases.
- **`net/http/httptest.NewServer`** is the only acceptable way to exercise the transport layer — see `transport_test.go`, `search_mock_test.go`, and `extract_mock_test.go` for the established pattern.
- New retry, error-classification, or rate-limit behavior must be covered by tests on **at least one of 401/429/500** response paths.
- Keep the file naming convention: `<feature>_test.go` for unit tests, `<feature>_mock_test.go` for tests that spin up an `httptest` server.

## Architecture conventions

The SDK has a deliberate layering — please preserve it when adding code:

- `client.go` — `Client`, `Option`, and the functional-options surface.
- `transport.go` — request construction, retry/backoff, header handling. Pure transport, no API knowledge.
- `errors.go` — `APIError`, `ErrorDetail`, sentinel errors.
- `search.go` / `extract.go` — typed request/response structs and the public `Client.Search` / `Client.Extract` methods.

Every new public API call:

1. Takes `ctx context.Context` as its first parameter.
2. Has a typed request struct and a typed response struct — no `map[string]any`, no pointer soup.
3. Comes with godoc on every exported type, field, and method.

## Pull requests

- Keep PRs small and focused — one concern per PR.
- CI (test, fmt, lint) must be green. We do not merge red branches.
- Conventional commit prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, …) are appreciated but not required.
- If you change behavior, update the godoc and, when relevant, the README.

## Reporting bugs

Open a GitHub issue with:

- The minimal reproduction (request + observed vs expected behavior).
- The SDK version (commit SHA or tag).
- The Go version and OS.
- If the issue surfaces as an `*APIError`, include `StatusCode`, `Kind`, and `TraceID` — `TraceID` lets us correlate with Kagi-side logs.
