// Advanced Search example: inline lens, filters, time window, per-request
// domain ranking, and in-line page extraction for the top results.
//
// Run: KAGI_API_KEY=... go run ./_examples/search-advanced
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

	req := kagi.SearchRequest{
		Query:    "errgroup pattern",
		Workflow: kagi.WorkflowSearch,
		Limit:    10,
		Lens: &kagi.Lens{
			SitesIncluded:    []string{"go.dev", "pkg.go.dev", "github.com"},
			KeywordsExcluded: []string{"tutorial"},
			TimeRelative:     kagi.TimeRelativeMonth,
		},
		Filters: &kagi.SearchFilters{
			Region: "us",
		},
		Personalizations: &kagi.Personalizations{
			Domains: []kagi.DomainRule{
				{Domain: "go.dev", Kind: kagi.DomainRuleRaise},
				{Domain: "w3schools.com", Kind: kagi.DomainRuleBlock},
			},
		},
		Extract: &kagi.SearchExtract{
			Count:   3,
			Timeout: 2,
		},
	}

	result, err := client.Search(ctx, req)
	if err != nil {
		log.Fatalf("search: %v", err)
	}

	fmt.Printf("trace=%s server_ms=%d hits=%d\n",
		result.Meta.Trace, result.Meta.MS, len(result.Data.Search))

	for i, hit := range result.Data.Search {
		fmt.Printf("\n[%d] %s\n    %s\n", i+1, hit.Title, hit.URL)
		snippet := hit.Snippet
		if len(snippet) > 240 {
			snippet = snippet[:240] + "..."
		}
		fmt.Printf("    %s\n", snippet)
	}

	for _, q := range result.Data.RelatedSearch {
		fmt.Printf("related: %s\n", q.Title)
	}
}
