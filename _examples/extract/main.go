// Minimal Extract example.
//
// Run: KAGI_API_KEY=... go run ./_examples/extract
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := client.Extract(ctx, kagi.ExtractRequest{
		Pages: []kagi.ExtractPage{
			{URL: "https://kagi.com/"},
			{URL: "https://help.kagi.com/kagi/api/search.html"},
		},
	})
	if err != nil {
		log.Fatalf("extract: %v", err)
	}

	fmt.Printf("trace=%s server_ms=%d\n", result.Meta.Trace, result.Meta.MS)

	for _, page := range result.Data {
		preview := page.Markdown
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Printf("\n--- %s ---\n%s\n", page.URL, preview)
	}

	for _, e := range result.Errors {
		fmt.Printf("\nerror: %s: %s\n", e.Location, e.Message)
	}
}
