// Package library proxies third-party character-card libraries so users
// can browse and import without leaving WuNest.
//
// V1 source: chub.ai (the main public collection). CHUB's search API is
// open and unauthenticated. Characters are distributed as V2/V3 character
// cards embedded in PNG — the same format we already parse for local
// uploads — so import reduces to "download PNG, call ParsePNGCard".
//
// Future sources slot in as additional methods on Client (Janitor, Risu,
// Pygmalion, AICC, etc.), each normalising into the same SearchResult /
// ImportedCard shapes.
package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shastitko1970-netizen/wunest/internal/characters"
)

// userAgent identifies WuNest to third-party services. CHUB rate-limits
// per-UA; having our own tag makes us distinguishable from generic bots.
const userAgent = "WuNest/0.1 (+https://nest.wusphere.ru)"

// Client is the HTTP gateway to the library providers.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// ─── Search ─────────────────────────────────────────────────────────

// SearchResult is one hit in the browse grid — trimmed to the fields
// the UI actually renders. We deliberately drop CHUB's denser payload
// (token label objects, forks count, etc.) to keep the wire small.
type SearchResult struct {
	FullPath     string    `json:"full_path"`
	Name         string    `json:"name"`
	Tagline      string    `json:"tagline,omitempty"`
	Description  string    `json:"description,omitempty"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	MaxResURL    string    `json:"max_res_url,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	StarCount    int       `json:"star_count"`
	RatingCount  int       `json:"rating_count"`
	Rating       float64   `json:"rating"`
	NSFW         bool      `json:"nsfw"`
	Creator      string    `json:"creator,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	LastActivity time.Time `json:"last_activity,omitempty"`
}

// SearchOptions drive the CHUB query. Defaults (empty zero-values) produce
// the "trending SFW characters" listing — a reasonable landing state.
type SearchOptions struct {
	Query         string
	Page          int    // 1-based
	PerPage       int    // capped to 48
	Sort          string // trending_downloads | created_at | last_activity_at | star_count
	IncludeNSFW   bool
	IncludeTags   []string // AND-match
	ExcludeTags   []string
}

// chubSearchResponse mirrors the subset of CHUB's JSON we consume.
type chubSearchResponse struct {
	Data struct {
		Count int        `json:"count"`
		Nodes []chubNode `json:"nodes"`
	} `json:"data"`
}

type chubNode struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	FullPath       string  `json:"fullPath"`
	Description    string  `json:"description"`
	StarCount      int     `json:"starCount"`
	LastActivityAt string  `json:"lastActivityAt"`
	CreatedAt      string  `json:"createdAt"`
	Topics         []string `json:"topics"`
	RatingCount    int     `json:"ratingCount"`
	Rating         float64 `json:"rating"`
	Tagline        string  `json:"tagline"`
	AvatarURL      string  `json:"avatar_url"`
	MaxResURL      string  `json:"max_res_url"`
	NSFWImage      bool    `json:"nsfw_image"`
}

// SearchChub runs a characters-namespace search against chub.ai.
func (c *Client) SearchChub(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 24
	}
	if perPage > 48 {
		perPage = 48
	}
	sort := opts.Sort
	if sort == "" {
		sort = "trending_downloads"
	}

	q := url.Values{}
	q.Set("namespace", "characters")
	q.Set("first", strconv.Itoa(perPage))
	q.Set("page", strconv.Itoa(page))
	q.Set("sort", sort)
	if opts.Query != "" {
		q.Set("search", opts.Query)
	}
	if opts.IncludeNSFW {
		q.Set("nsfw", "true")
	} else {
		q.Set("nsfw", "false")
	}
	if len(opts.IncludeTags) > 0 {
		q.Set("tags", strings.Join(opts.IncludeTags, ","))
	}
	if len(opts.ExcludeTags) > 0 {
		q.Set("exclude_tags", strings.Join(opts.ExcludeTags, ","))
	}

	endpoint := "https://api.chub.ai/search?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("chub search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, 0, fmt.Errorf("chub search returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed chubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, fmt.Errorf("decode chub search: %w", err)
	}

	out := make([]SearchResult, 0, len(parsed.Data.Nodes))
	for _, n := range parsed.Data.Nodes {
		creator := ""
		if i := strings.Index(n.FullPath, "/"); i > 0 {
			creator = n.FullPath[:i]
		}
		out = append(out, SearchResult{
			FullPath:     n.FullPath,
			Name:         n.Name,
			Tagline:      n.Tagline,
			Description:  n.Description,
			AvatarURL:    n.AvatarURL,
			MaxResURL:    n.MaxResURL,
			Tags:         n.Topics,
			StarCount:    n.StarCount,
			RatingCount:  n.RatingCount,
			Rating:       n.Rating,
			NSFW:         n.NSFWImage,
			Creator:      creator,
			CreatedAt:    parseTime(n.CreatedAt),
			LastActivity: parseTime(n.LastActivityAt),
		})
	}
	return out, parsed.Data.Count, nil
}

// ─── Import ─────────────────────────────────────────────────────────

// ImportedCard is the result of fetching + parsing a character from a
// third-party library. Ready to drop into characters.Repository.Create.
type ImportedCard struct {
	Name      string
	Data      characters.CharacterData
	Spec      string // "chara_card_v3" after ParsePNGCard normalises
	AvatarURL string // pass-through CDN URL (we don't re-host for v1)
	SourceURL string // canonical CHUB URL for attribution
	Tags      []string
}

// ImportChub downloads the V2/V3 PNG at CHUB's max_res_url for the given
// fullPath, runs it through our existing PNG parser, and returns a
// normalised ImportedCard. The caller (handler) is responsible for turning
// that into a nest_characters row and attributing it to a user.
func (c *Client) ImportChub(ctx context.Context, fullPath string) (*ImportedCard, error) {
	// Every CHUB character has a predictable PNG path under avatars.charhub.io.
	// We could instead hit /api/characters/{creator}/{slug}?full=true and build
	// the card from JSON, but downloading the PNG means we share the exact
	// import path as a manual upload — any bugs in one affect the other,
	// which is a feature.
	pngURL := "https://avatars.charhub.io/avatars/" + fullPath + "/chara_card_v2.png"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pngURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/png")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chub download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chub download returned %d", resp.StatusCode)
	}

	// Cap at 16 MiB — same ceiling as our manual-upload handler.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read chub png: %w", err)
	}

	data, spec, err := characters.ParsePNGCard(body)
	if err != nil {
		return nil, fmt.Errorf("parse chub png: %w", err)
	}

	return &ImportedCard{
		Name:      data.Name,
		Data:      *data,
		Spec:      spec,
		AvatarURL: "https://avatars.charhub.io/avatars/" + fullPath + "/avatar.webp",
		SourceURL: "https://chub.ai/characters/" + fullPath,
		Tags:      data.Tags,
	}, nil
}

// parseTime tolerates CHUB's RFC3339 strings and returns zero on garbage.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
