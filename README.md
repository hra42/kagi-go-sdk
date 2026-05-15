# kagi-go-sdk

A handbuilt, idiomatic Go SDK for the [Kagi Search API](https://help.kagi.com/kagi/api/overview.html).

Designed to be ergonomic, production-ready, and easy to vendor — covering the full API surface with clean types, retry logic, rate limit handling, and proper context propagation.

> **Status:** early development.

## Why this SDK

- **Zero external dependencies.** Standard library only — `go.mod` has no `require` entries beyond the Go version itself. Safe to vendor, trivial to audit.
- **Idiomatic Go.** Functional options, typed request/response structs, `context.Context` on every call, `errors.As`-compatible error hierarchy.
- **Production-ready.** Transparent retries with exponential backoff + jitter, `Retry-After` honoring, exposed rate-limit headers.
- **Handwritten, not generated.** Clean field names, real godoc, no pointer soup.

## Install

```sh
go get github.com/hra42/kagi-go-sdk
```

Requires Go 1.26+.

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

	for _, r := range res.Results {
		fmt.Printf("%s\n  %s\n\n", r.Title, r.URL)
	}
}
```

## Configuration

The client uses the functional options pattern:

```go
client := kagi.NewClient(apiKey,
	kagi.WithTimeout(30 * time.Second),
	kagi.WithRetries(3),
	kagi.WithUserAgent("my-app/1.0"),
	kagi.WithHTTPClient(myHTTPClient),
	kagi.WithBaseURL("https://kagi.com/api/v0"),
)
```

## Error handling

All errors implement the standard `error` interface and unwrap cleanly:

```go
res, err := client.Search(ctx, req)
if err != nil {
	var rl *kagi.ErrRateLimited
	if errors.As(err, &rl) {
		log.Printf("rate limited, retry after %s", rl.RetryAfter)
		return
	}
	return err
}
```

Available error types: `ErrUnauthorized`, `ErrRateLimited`, `ErrBadRequest`, `ErrServerError`.

## Endpoints

| Method | Description |
|---|---|
| `client.Search(ctx, SearchRequest)` | Kagi Search API |
| `client.Extract(ctx, ExtractRequest)` | URL content extraction (single or batch) |

See `_examples/` for runnable usage.

## Contributing

The hard rule: **zero external dependencies, including in tests.** Use `net/http/httptest` and the stdlib `testing` package only — no `testify`, `gomock`, or third-party assertions. Table-driven tests preferred.

```sh
go test ./...
go vet ./...
gofmt -s -w .
```

## License

TBD.
