package kagi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleSearchResponse = `{
  "meta": {
    "trace": "69c3f5c4168f66b860e951c585550f1c",
    "node": "us-central1",
    "ms": 213
  },
  "data": {
    "search": [
      {
        "url": "https://example.com/jobs",
        "title": "Steve Jobs",
        "snippet": "co-founder of Apple",
        "time": "2017-01-09T14:49:00Z",
        "image": {"url": "https://example.com/i.jpg", "height": 1024, "width": 1548},
        "props": {"paywalled": true, "sort_group_id": "example.com"}
      }
    ],
    "related_search": [
      {"url": "https://kagi.com/search?q=steve+wozniak", "title": "steve wozniak"}
    ]
  }
}`

func TestSearchHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("path = %q, want /api/v1/search", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["query"] != "steve jobs" {
			t.Errorf("query = %v, want %q", body["query"], "steve jobs")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, sampleSearchResponse)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
	)

	res, err := client.Search(context.Background(), SearchRequest{Query: "steve jobs"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Meta.Trace != "69c3f5c4168f66b860e951c585550f1c" {
		t.Errorf("Meta.Trace = %q", res.Meta.Trace)
	}
	if res.Meta.Node != "us-central1" {
		t.Errorf("Meta.Node = %q", res.Meta.Node)
	}
	if res.Meta.MS != 213 {
		t.Errorf("Meta.MS = %d", res.Meta.MS)
	}
	if got := len(res.Data.Search); got != 1 {
		t.Fatalf("Data.Search len = %d, want 1", got)
	}
	hit := res.Data.Search[0]
	if hit.URL != "https://example.com/jobs" {
		t.Errorf("URL = %q", hit.URL)
	}
	if hit.Title != "Steve Jobs" {
		t.Errorf("Title = %q", hit.Title)
	}
	if hit.Image == nil || hit.Image.Height != 1024 || hit.Image.Width != 1548 {
		t.Errorf("Image = %+v", hit.Image)
	}
	if len(hit.Props) == 0 {
		t.Fatal("Props is empty, want raw JSON object")
	}
	var props map[string]any
	if err := json.Unmarshal(hit.Props, &props); err != nil {
		t.Fatalf("Props is not valid JSON: %v", err)
	}
	if props["paywalled"] != true {
		t.Errorf("Props[paywalled] = %v", props["paywalled"])
	}
	if got := len(res.Data.RelatedSearch); got != 1 {
		t.Errorf("Data.RelatedSearch len = %d, want 1", got)
	}
}

func TestSearchFullRequestBody(t *testing.T) {
	safe := false
	req := SearchRequest{
		Query:    "steve jobs",
		Workflow: WorkflowImages,
		LensID:   "abc123",
		Lens: &Lens{
			SitesIncluded:    []string{"example.com"},
			SitesExcluded:    []string{"spam.example"},
			KeywordsIncluded: []string{"apple"},
			KeywordsExcluded: []string{"pear"},
			FileType:         "pdf",
			TimeAfter:        "2020-01-01",
			TimeBefore:       "2024-01-01",
			TimeRelative:     TimeRelativeMonth,
			SearchRegion:     "US",
		},
		Filters: &SearchFilters{
			Region: "DE",
			After:  "2021-01-01",
			Before: "2023-01-01",
		},
		Extract: &SearchExtract{Count: 3, Timeout: 1.5},
		Personalizations: &Personalizations{
			Domains: []DomainRule{{Domain: "example.com", Kind: DomainRuleRaise}},
			Regexes: []RegexRule{{Regex: `^https?://(www\.)?reddit\.com.*`, Replacement: "https://old.reddit.com"}},
		},
		Page:       2,
		Limit:      25,
		Timeout:    2.5,
		SafeSearch: &safe,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if got["query"] != "steve jobs" {
			t.Errorf("query = %v", got["query"])
		}
		if got["workflow"] != "images" {
			t.Errorf("workflow = %v", got["workflow"])
		}
		if got["lens_id"] != "abc123" {
			t.Errorf("lens_id = %v", got["lens_id"])
		}
		if got["safe_search"] != false {
			t.Errorf("safe_search = %v, want false", got["safe_search"])
		}
		if got["page"].(float64) != 2 {
			t.Errorf("page = %v", got["page"])
		}
		if got["limit"].(float64) != 25 {
			t.Errorf("limit = %v", got["limit"])
		}
		if got["timeout"].(float64) != 2.5 {
			t.Errorf("timeout = %v", got["timeout"])
		}

		lens, ok := got["lens"].(map[string]any)
		if !ok {
			t.Fatalf("lens missing or wrong type: %T", got["lens"])
		}
		if lens["file_type"] != "pdf" {
			t.Errorf("lens.file_type = %v", lens["file_type"])
		}
		if lens["time_relative"] != "month" {
			t.Errorf("lens.time_relative = %v", lens["time_relative"])
		}
		if lens["search_region"] != "US" {
			t.Errorf("lens.search_region = %v", lens["search_region"])
		}
		if sites, _ := lens["sites_included"].([]any); len(sites) != 1 || sites[0] != "example.com" {
			t.Errorf("lens.sites_included = %v", lens["sites_included"])
		}

		filters, ok := got["filters"].(map[string]any)
		if !ok {
			t.Fatalf("filters missing")
		}
		if filters["region"] != "DE" || filters["after"] != "2021-01-01" || filters["before"] != "2023-01-01" {
			t.Errorf("filters = %v", filters)
		}

		extract, ok := got["extract"].(map[string]any)
		if !ok {
			t.Fatalf("extract missing")
		}
		if extract["count"].(float64) != 3 || extract["timeout"].(float64) != 1.5 {
			t.Errorf("extract = %v", extract)
		}

		pers, ok := got["personalizations"].(map[string]any)
		if !ok {
			t.Fatalf("personalizations missing")
		}
		domains, _ := pers["domains"].([]any)
		if len(domains) != 1 {
			t.Fatalf("personalizations.domains len = %d", len(domains))
		}
		d0 := domains[0].(map[string]any)
		if d0["domain"] != "example.com" || d0["kind"] != "raise" {
			t.Errorf("domains[0] = %v", d0)
		}
		regexes, _ := pers["regexes"].([]any)
		if len(regexes) != 1 {
			t.Fatalf("personalizations.regexes len = %d", len(regexes))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"data":{}}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	if _, err := client.Search(context.Background(), req); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestSearchEmptyQueryRejected(t *testing.T) {
	cases := []string{"", "   ", "\t\n"}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Fatal("server should not be called when Query is empty")
			}))
			defer server.Close()

			client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
			res, err := client.Search(context.Background(), SearchRequest{Query: q})
			if err == nil {
				t.Fatal("expected error for empty query")
			}
			if res != nil {
				t.Errorf("expected nil result, got %+v", res)
			}
			if !strings.Contains(err.Error(), "Query") {
				t.Errorf("error = %q, want to mention Query", err.Error())
			}
		})
	}
}

func TestSearchRequestOmitemptyShape(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{},"data":{}}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	if _, err := client.Search(context.Background(), SearchRequest{Query: "hello"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(capturedBody) != 1 {
		t.Fatalf("body keys = %v, want exactly [query]", keys(capturedBody))
	}
	if capturedBody["query"] != "hello" {
		t.Errorf("query = %v", capturedBody["query"])
	}
}

func TestSearchUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"error":[{"code":"unauthorized","url":"https://help.kagi.com/api/errors#unauthorized","message":"bad key"}]}`)
	}))
	defer server.Close()

	client := NewClient("bad", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	res, err := client.Search(context.Background(), SearchRequest{Query: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %+v", res)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(ErrUnauthorized) failed: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*APIError) failed: %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
}

func TestSearchContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called with cancelled context")
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Search(ctx, SearchRequest{Query: "x"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(context.Canceled) failed: %v", err)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
