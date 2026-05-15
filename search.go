package kagi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Workflow selects which class of results the Search endpoint should return.
type Workflow string

// Supported [Workflow] values.
const (
	WorkflowSearch   Workflow = "search"
	WorkflowImages   Workflow = "images"
	WorkflowVideos   Workflow = "videos"
	WorkflowNews     Workflow = "news"
	WorkflowPodcasts Workflow = "podcasts"
)

// TimeRelative restricts results to pages updated within a recent window.
type TimeRelative string

// Supported [TimeRelative] values.
const (
	TimeRelativeDay   TimeRelative = "day"
	TimeRelativeWeek  TimeRelative = "week"
	TimeRelativeMonth TimeRelative = "month"
)

// DomainRuleKind is the handling mode applied to a [DomainRule].
type DomainRuleKind string

// Supported [DomainRuleKind] values.
const (
	DomainRuleBlock DomainRuleKind = "block"
	DomainRuleLower DomainRuleKind = "lower"
	DomainRuleRaise DomainRuleKind = "raise"
	DomainRulePin   DomainRuleKind = "pin"
)

// SearchRequest is the input to [Client.Search].
//
// Only [SearchRequest.Query] is required; all other fields are optional and
// are omitted from the request body when left at their zero value.
type SearchRequest struct {
	// Query is the search query to run. Required.
	Query string `json:"query"`

	// Workflow selects the category of results. Defaults to
	// [WorkflowSearch] server-side when unset.
	Workflow Workflow `json:"workflow,omitempty"`

	// LensID applies a saved or built-in lens by identifier. Accepts either
	// the bare ID portion of a lens URL (https://kagi.com/lenses/ID) or the
	// full URL.
	LensID string `json:"lens_id,omitempty"`

	// Lens describes an inline lens to apply for this request. Options
	// supplied by the lens take precedence over equivalent operators in the
	// raw query string.
	Lens *Lens `json:"lens,omitempty"`

	// Filters narrow results by region or publication date. Filters take
	// priority over equivalent fields on Lens.
	Filters *SearchFilters `json:"filters,omitempty"`

	// Extract opts in to fetching page content for the top results. Use of
	// this option incurs additional cost billed at the account's Extract
	// API rate.
	Extract *SearchExtract `json:"extract,omitempty"`

	// Personalizations applies per-request domain and regex ranking rules.
	Personalizations *Personalizations `json:"personalizations,omitempty"`

	// Page is the 1-indexed page number for paginated results. Valid range
	// is 1..10 server-side.
	Page int `json:"page,omitempty"`

	// Limit caps the number of results returned. Valid range is 1..1024
	// server-side. Note: this only limits what is returned, it does not
	// reduce the amount of work the server does.
	Limit int `json:"limit,omitempty"`

	// Timeout is the server-side time budget for collecting results, in
	// seconds. Valid range is 0.5..4 server-side.
	Timeout float64 `json:"timeout,omitempty"`

	// SafeSearch toggles filtering of potentially NSFW content. The server
	// default is true; leave nil to inherit it, or set explicitly to
	// override.
	SafeSearch *bool `json:"safe_search,omitempty"`
}

// Lens is an inline description of a search lens.
//
// A lens restricts or shapes the result set without altering the query. See
// the Kagi lenses documentation for behavior of each field.
type Lens struct {
	// SitesIncluded restricts results to these domains.
	SitesIncluded []string `json:"sites_included,omitempty"`
	// SitesExcluded removes results from these domains.
	SitesExcluded []string `json:"sites_excluded,omitempty"`
	// KeywordsIncluded requires results to contain these keywords.
	KeywordsIncluded []string `json:"keywords_included,omitempty"`
	// KeywordsExcluded removes results containing these keywords.
	KeywordsExcluded []string `json:"keywords_excluded,omitempty"`
	// FileType narrows to a specific file type (for example, "pdf").
	FileType string `json:"file_type,omitempty"`
	// TimeAfter restricts to pages updated on or after this date
	// (YYYY-MM-DD).
	TimeAfter string `json:"time_after,omitempty"`
	// TimeBefore restricts to pages updated on or before this date
	// (YYYY-MM-DD).
	TimeBefore string `json:"time_before,omitempty"`
	// TimeRelative restricts to a recent window relative to today.
	TimeRelative TimeRelative `json:"time_relative,omitempty"`
	// SearchRegion requests results localized to a region. Use an
	// ISO-3166-1 alpha-2 country code, or the special value "no_region".
	SearchRegion string `json:"search_region,omitempty"`
}

// SearchFilters applies coarse-grained filters to search results. Fields here
// take priority over equivalent fields on [Lens].
type SearchFilters struct {
	// Region filters results to an ISO-3166-1 alpha-2 country code.
	Region string `json:"region,omitempty"`
	// After filters to results published or updated on or after this date
	// (YYYY-MM-DD).
	After string `json:"after,omitempty"`
	// Before filters to results published or updated on or before this date
	// (YYYY-MM-DD).
	Before string `json:"before,omitempty"`
}

// SearchExtract configures optional in-line page content extraction for
// search results. When set, the server extracts content from the top results
// and embeds it into each result's snippet.
type SearchExtract struct {
	// Count is the number of top results to extract content from. Valid
	// range is 1..10 server-side.
	Count int `json:"count,omitempty"`
	// Timeout is a per-page extraction time budget in seconds, independent
	// of the top-level search timeout. Valid range is 0.5..4 server-side.
	Timeout float64 `json:"timeout,omitempty"`
}

// Personalizations customizes result ranking for a single request.
type Personalizations struct {
	// Domains carries per-domain ranking adjustments (up to 1000 rules).
	Domains []DomainRule `json:"domains,omitempty"`
	// Regexes carries regex-based URL rewriting rules (up to 1000 rules,
	// max 1000 bytes per pattern).
	Regexes []RegexRule `json:"regexes,omitempty"`
}

// DomainRule adjusts ranking for a domain pattern.
type DomainRule struct {
	// Domain is the pattern to match, for example "example.com" or a TLD
	// suffix such as ".co.uk".
	Domain string `json:"domain"`
	// Kind is how the matched domain should be handled.
	Kind DomainRuleKind `json:"kind"`
}

// RegexRule rewrites matching URLs in results.
type RegexRule struct {
	// Regex is the pattern to match against the result URL.
	Regex string `json:"regex"`
	// Replacement is applied when Regex matches. Capture groups may be
	// referenced as "$1", "$2", and so on. Paths and query parameters are
	// preserved when not overwritten.
	Replacement string `json:"replacement,omitempty"`
}

// SearchResult is the response returned by [Client.Search].
type SearchResult struct {
	// Meta carries trace and timing information about the request.
	Meta SearchMeta
	// Data carries result buckets, grouped by category. Any bucket may be
	// absent or empty for a given query.
	Data SearchData
}

// SearchMeta is the meta envelope of a search response.
//
// The exact set of fields is intended for debugging and may evolve over time;
// callers should treat it as advisory and avoid building hard dependencies
// on individual fields.
type SearchMeta struct {
	// Trace is the request trace ID, useful when contacting Kagi support.
	Trace string `json:"trace"`
	// Node identifies the server node that fulfilled the request.
	Node string `json:"node"`
	// MS is the server-side processing time in milliseconds, excluding
	// network round-trip.
	MS int `json:"ms"`
}

// SearchData groups search results by category. Each bucket holds zero or
// more [SearchHit] entries with category-specific extra data exposed via
// [SearchHit.Props].
type SearchData struct {
	// Search holds general web page results.
	Search []SearchHit `json:"search,omitempty"`
	// Image holds image results.
	Image []SearchHit `json:"image,omitempty"`
	// Video holds video results.
	Video []SearchHit `json:"video,omitempty"`
	// News holds news article results.
	News []SearchHit `json:"news,omitempty"`
	// Podcast holds podcast episode results.
	Podcast []SearchHit `json:"podcast,omitempty"`
	// PodcastCreator holds results for creators of podcasts.
	PodcastCreator []SearchHit `json:"podcast_creator,omitempty"`
	// AdjacentQuestion holds results gathered by asking related questions
	// alongside the original query. The associated question is exposed via
	// each hit's Props ("question" field).
	AdjacentQuestion []SearchHit `json:"adjacent_question,omitempty"`
	// DirectAnswer holds quick answers for queries like math expressions
	// or unit conversions.
	DirectAnswer []SearchHit `json:"direct_answer,omitempty"`
	// InterestingNews holds news pulled from Kagi's curated news index.
	InterestingNews []SearchHit `json:"interesting_news,omitempty"`
	// InterestingFinds holds small-web results from Kagi's small-web
	// index.
	InterestingFinds []SearchHit `json:"interesting_finds,omitempty"`
	// Infobox holds summary entries for a person, place, or thing.
	Infobox []SearchHit `json:"infobox,omitempty"`
	// Code holds results pointing to code resources or repositories.
	Code []SearchHit `json:"code,omitempty"`
	// PackageTracking holds tracking-site results for shipment numbers.
	PackageTracking []SearchHit `json:"package_tracking,omitempty"`
	// PublicRecords holds results for public records such as government
	// documents.
	PublicRecords []SearchHit `json:"public_records,omitempty"`
	// Weather holds current-weather results.
	Weather []SearchHit `json:"weather,omitempty"`
	// RelatedSearch holds queries related to the current one.
	RelatedSearch []SearchHit `json:"related_search,omitempty"`
	// Listicle holds list-style results.
	Listicle []SearchHit `json:"listicle,omitempty"`
	// WebArchive holds archived-website results.
	WebArchive []SearchHit `json:"web_archive,omitempty"`
}

// SearchHit is a single search result. The shared fields cover all result
// categories; category-specific extras are preserved as raw JSON in
// [SearchHit.Props] and can be decoded by the caller when needed.
type SearchHit struct {
	// URL is the direct URL of the matched resource.
	URL string `json:"url"`
	// Title is the resource title. For HTML pages this reflects the
	// document's <title>; for videos it's the display title on the source
	// site.
	Title string `json:"title"`
	// Snippet is a short summary of the resource. When in-line extraction
	// was requested via [SearchRequest.Extract], this field is replaced
	// with the extracted page markdown.
	Snippet string `json:"snippet,omitempty"`
	// Time is the resource's creation or last-updated timestamp as
	// returned by the server (typically an RFC-3339 string).
	Time string `json:"time,omitempty"`
	// Image is the cover image or video thumbnail for the resource, when
	// available.
	Image *Image `json:"image,omitempty"`
	// Props is a category-specific bag of additional metadata. Its shape
	// varies by result category (for example, "question" on
	// AdjacentQuestion hits, "paywalled" on Search hits). Decode with
	// json.Unmarshal when needed.
	Props json.RawMessage `json:"props,omitempty"`
}

// Image describes an image associated with a [SearchHit].
type Image struct {
	// URL links directly to the image bytes.
	URL string `json:"url"`
	// Height is the image height in pixels, when reported by the server.
	Height int `json:"height,omitempty"`
	// Width is the image width in pixels, when reported by the server.
	Width int `json:"width,omitempty"`
}

// Search performs a web search.
//
// SearchRequest.Query is required; an empty or whitespace-only query is
// rejected without contacting the server. The returned [SearchResult]
// groups hits by category in [SearchData]; any category may be absent for a
// given query.
//
// API errors are returned as a [*APIError] wrapping one of the package's
// sentinel errors (for example, [ErrUnauthorized] or [ErrRateLimited]); use
// [errors.Is] and [errors.As] to classify and inspect them.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("kagi: SearchRequest.Query is required")
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/search", nil, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	var envelope struct {
		Meta SearchMeta `json:"meta"`
		Data SearchData `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("kagi: decode search response: %w", err)
	}

	return &SearchResult{Meta: envelope.Meta, Data: envelope.Data}, nil
}
