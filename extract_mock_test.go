//go:build mockserver

package kagi

import (
	"context"
	"testing"
	"time"
)

// TestExtractAgainstRedoclyMock hits the Redocly-hosted Kagi OpenAPI mock
// server to confirm ExtractRequest/ExtractResult round-trip the spec's
// example payload without losing fields. This is not a live API call — no
// auth, no billing, just the spec's canned example.
//
// Opt in with:
//
//	go test -tags=mockserver -run TestExtractAgainstRedoclyMock -v
func TestExtractAgainstRedoclyMock(t *testing.T) {
	client := NewClient("mock-key",
		WithBaseURL("https://kagi.redocly.app/_mock/openapi"),
		WithTimeout(15*time.Second),
	)

	res, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{
			{URL: "https://example.com/article1"},
			{URL: "https://example.com/article2"},
		},
		Timeout: 1.337,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if res.Meta.Trace == "" {
		t.Error("Meta.Trace is empty")
	}
	if len(res.Data) == 0 {
		t.Fatal("Data is empty")
	}
	for i, p := range res.Data {
		if p.URL == "" {
			t.Errorf("Data[%d].URL is empty", i)
		}
		if p.Markdown == "" {
			t.Errorf("Data[%d].Markdown is empty", i)
		}
	}
}
