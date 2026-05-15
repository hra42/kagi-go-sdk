// Minimal Search example.
//
// Run: KAGI_API_KEY=... go run ./_examples/search
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	kagi "github.com/hra42/kagi-go-sdk"
)

func main() {
	apiKey := os.Getenv("KAGI_API_KEY")
	if apiKey == "" {
		log.Fatal("KAGI_API_KEY is not set")
	}

	client := kagi.NewClient(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Search(ctx, kagi.SearchRequest{
		Query: "golang context cancellation",
		Limit: 5,
	})
	if err != nil {
		log.Fatalf("search: %v", err)
	}

	fmt.Printf("trace=%s server_ms=%d\n", result.Meta.Trace, result.Meta.MS)
	for i, hit := range result.Data.Search {
		fmt.Printf("\n[%d] %s\n    %s\n    %s\n", i+1, hit.Title, hit.URL, hit.Snippet)
	}
}
