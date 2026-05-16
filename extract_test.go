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

const sampleExtractResponse = `{
  "meta": {
    "trace": "abc123def456",
    "node": "us-east-1",
    "ms": 1250
  },
  "data": [
    {"url": "https://example.com/article1", "markdown": "# Article 1\n\nBody one."},
    {"url": "https://example.com/article2", "markdown": "# Article 2\n\nBody two."}
  ]
}`

func TestExtractHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/extract" {
			t.Errorf("path = %q, want /api/v1/extract", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		pages, ok := body["pages"].([]any)
		if !ok || len(pages) != 2 {
			t.Fatalf("pages = %v", body["pages"])
		}
		first, _ := pages[0].(map[string]any)
		if first["url"] != "https://example.com/article1" {
			t.Errorf("pages[0].url = %v", first["url"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, sampleExtractResponse)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL+"/api/v1"),
		WithHTTPClient(server.Client()),
	)

	res, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{
			{URL: "https://example.com/article1"},
			{URL: "https://example.com/article2"},
		},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Meta.Trace != "abc123def456" {
		t.Errorf("Meta.Trace = %q", res.Meta.Trace)
	}
	if res.Meta.Node != "us-east-1" {
		t.Errorf("Meta.Node = %q", res.Meta.Node)
	}
	if res.Meta.MS != 1250 {
		t.Errorf("Meta.MS = %d", res.Meta.MS)
	}
	if got := len(res.Data); got != 2 {
		t.Fatalf("Data len = %d, want 2", got)
	}
	if res.Data[0].URL != "https://example.com/article1" {
		t.Errorf("Data[0].URL = %q", res.Data[0].URL)
	}
	if !strings.Contains(res.Data[0].Markdown, "Article 1") {
		t.Errorf("Data[0].Markdown = %q", res.Data[0].Markdown)
	}
	if len(res.Errors) != 0 {
		t.Errorf("Errors = %+v, want empty", res.Errors)
	}
}

func TestExtractFullRequestBody(t *testing.T) {
	req := ExtractRequest{
		Pages: []ExtractPage{
			{URL: "https://example.com/a"},
			{URL: "https://example.com/b"},
		},
		Timeout: 1.337,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got["timeout"].(float64) != 1.337 {
			t.Errorf("timeout = %v", got["timeout"])
		}
		if _, ok := got["format"]; ok {
			t.Errorf("format should never be sent, got %v", got["format"])
		}
		pages, _ := got["pages"].([]any)
		if len(pages) != 2 {
			t.Fatalf("pages len = %d", len(pages))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"data":[]}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	if _, err := client.Extract(context.Background(), req); err != nil {
		t.Fatalf("Extract: %v", err)
	}
}

func TestExtractRequestOmitemptyShape(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{},"data":[]}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	_, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{{URL: "https://example.com/x"}},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(capturedBody) != 1 {
		t.Fatalf("body keys = %v, want exactly [pages]", keys(capturedBody))
	}
	if _, ok := capturedBody["pages"]; !ok {
		t.Errorf("missing pages key, got %v", capturedBody)
	}
}

func TestExtractEmptyPagesRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called when Pages is empty")
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	res, err := client.Extract(context.Background(), ExtractRequest{})
	if err == nil {
		t.Fatal("expected error for empty Pages")
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	if !strings.Contains(err.Error(), "Pages") {
		t.Errorf("error = %q, want to mention Pages", err.Error())
	}
}

func TestExtractBlankURLRejected(t *testing.T) {
	cases := []string{"", "   ", "\t\n"}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Fatal("server should not be called when URL is blank")
			}))
			defer server.Close()

			client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
			res, err := client.Extract(context.Background(), ExtractRequest{
				Pages: []ExtractPage{{URL: "https://ok.example/"}, {URL: u}},
			})
			if err == nil {
				t.Fatal("expected error for blank URL")
			}
			if res != nil {
				t.Errorf("expected nil result, got %+v", res)
			}
			if !strings.Contains(err.Error(), "Pages[1]") {
				t.Errorf("error = %q, want to mention Pages[1]", err.Error())
			}
		})
	}
}

func TestExtractPartialSuccess(t *testing.T) {
	const body = `{
	  "meta": {"trace": "t", "ms": 42},
	  "data": [
	    {"url": "https://ok.example/", "markdown": "# OK"},
	    {"url": "https://broken.example/"}
	  ],
	  "errors": [
	    {"code": "extract.invalid_url", "url": "https://help.kagi.com/api/errors#extract.invalid_url", "message": "fetch failed", "location": "pages[1].url"}
	  ]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	res, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{{URL: "https://ok.example/"}, {URL: "https://broken.example/"}},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Data) != 2 {
		t.Fatalf("Data len = %d", len(res.Data))
	}
	if res.Data[1].Markdown != "" {
		t.Errorf("Data[1].Markdown = %q, want empty", res.Data[1].Markdown)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("Errors len = %d", len(res.Errors))
	}
	if res.Errors[0].Code != "extract.invalid_url" {
		t.Errorf("Errors[0].Code = %q", res.Errors[0].Code)
	}
	if res.Errors[0].Location != "pages[1].url" {
		t.Errorf("Errors[0].Location = %q", res.Errors[0].Location)
	}
}

func TestExtractUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"error":[{"code":"unauthorized","url":"https://help.kagi.com/api/errors#unauthorized","message":"bad key"}]}`)
	}))
	defer server.Close()

	client := NewClient("bad", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	res, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{{URL: "https://example.com/"}},
	})
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

func TestExtractRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"error":[{"code":"rate_limited","url":"https://help.kagi.com/api/errors#rate_limited"}]}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()), WithRetries(0))
	_, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{{URL: "https://example.com/"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("errors.Is(ErrRateLimited) failed: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*APIError) failed: %v", err)
	}
	if apiErr.RetryAfter == 0 {
		t.Errorf("RetryAfter = 0, want non-zero")
	}
}

func TestExtractServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"meta":{"trace":"t"},"error":[{"code":"server_error","url":"https://help.kagi.com/api/errors#server_error"}]}`)
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()), WithRetries(0))
	_, err := client.Extract(context.Background(), ExtractRequest{
		Pages: []ExtractPage{{URL: "https://example.com/"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrServerError) {
		t.Errorf("errors.Is(ErrServerError) failed: %v", err)
	}
}

func TestExtractContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called with cancelled context")
	}))
	defer server.Close()

	client := NewClient("k", WithBaseURL(server.URL+"/api/v1"), WithHTTPClient(server.Client()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Extract(ctx, ExtractRequest{
		Pages: []ExtractPage{{URL: "https://example.com/"}},
	})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(context.Canceled) failed: %v", err)
	}
}
