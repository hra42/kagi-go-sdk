//go:build mockserver

package kagi

import (
	"context"
	"testing"
	"time"
)

// TestSearchAgainstRedoclyMock hits the Redocly-hosted Kagi OpenAPI mock
// server to confirm SearchRequest/SearchResult round-trip the real example
// payload without losing fields. This is not a live API call — no auth, no
// billing, just the spec's canned example.
//
// Opt in with:
//
//	go test -tags=mockserver -run TestSearchAgainstRedoclyMock -v
func TestSearchAgainstRedoclyMock(t *testing.T) {
	client := NewClient("mock-key",
		WithBaseURL("https://kagi.redocly.app/_mock/openapi"),
		WithTimeout(15*time.Second),
	)

	res, err := client.Search(context.Background(), SearchRequest{Query: "steve jobs"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if res.Meta.Trace == "" {
		t.Error("Meta.Trace is empty")
	}
	if len(res.Data.Search) == 0 {
		t.Error("Data.Search is empty")
	}
	if len(res.Data.Infobox) == 0 {
		t.Error("Data.Infobox is empty")
	}
	if len(res.Data.AdjacentQuestion) == 0 {
		t.Error("Data.AdjacentQuestion is empty")
	}
	if len(res.Data.WebArchive) == 0 {
		t.Error("Data.WebArchive is empty")
	}
	for i, h := range res.Data.Search {
		if h.URL == "" || h.Title == "" {
			t.Errorf("Data.Search[%d] missing URL or Title: %+v", i, h)
		}
		if len(h.Props) == 0 {
			t.Errorf("Data.Search[%d] Props is empty", i)
		}
	}
}
