// Custom-client example: compose NewClient with every supported Option,
// including a caller-supplied *http.Client.
//
// Run: KAGI_API_KEY=... go run ./_examples/custom-client
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	kagi "github.com/hra42/kagi-go-sdk"
)

func main() {
	apiKey := os.Getenv("KAGI_API_KEY")
	if apiKey == "" {
		log.Fatal("KAGI_API_KEY is not set")
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	client := kagi.NewClient(
		apiKey,
		kagi.WithHTTPClient(httpClient),
		kagi.WithTimeout(20*time.Second),
		kagi.WithRetries(5),
		kagi.WithBackoff(250*time.Millisecond, 10*time.Second),
		kagi.WithUserAgent("my-app/1.0 (+https://example.com)"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Search(ctx, kagi.SearchRequest{
		Query: "kagi search api",
		Limit: 3,
	})
	if err != nil {
		var apiErr *kagi.APIError
		if errors.As(err, &apiErr) {
			log.Fatalf("api error: status=%d trace=%s kind=%v details=%d",
				apiErr.StatusCode, apiErr.TraceID, apiErr.Kind, len(apiErr.Details))
		}
		log.Fatalf("search: %v", err)
	}

	fmt.Printf("ok: %d hits, server_ms=%d\n", len(result.Data.Search), result.Meta.MS)
	for _, hit := range result.Data.Search {
		fmt.Printf("- %s — %s\n", hit.Title, hit.URL)
	}
}
