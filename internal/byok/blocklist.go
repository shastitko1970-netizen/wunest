package byok

import (
	"net/url"
	"strings"
)

// Provider blocklist for BYOK keys.
//
// We refuse to store API keys for services we've identified as
// fraudulent or operating in bad faith. The block runs at the
// `POST /api/byok` create handler — independent of the user's
// `provider` selection — by matching the URL's host against the list
// below. Subdomains match too (e.g. `api.ellyai.pro` is blocked when
// `ellyai.pro` is on the list).
//
// Adding an entry:
//
//	{Host: "example.com", Label: "Example AI", Message: "..."}
//
// Removing an entry: just delete the row. No DB migration involved —
// the list is in code so changes ship with the next deploy.
type blockedProvider struct {
	// Host is the bare host (no scheme, no path). Match is exact OR
	// "subdomain of" — `api.ellyai.pro` matches `ellyai.pro`.
	Host string
	// Label is the user-visible name we put in the error response.
	Label string
	// Message is the explanation shown in the SPA's banner. Russian
	// because that's the dominant locale; the SPA can still localise
	// via its own copy if/when EN is needed (the `kind` field is the
	// stable hook).
	Message string
}

var blockedProviders = []blockedProvider{
	{
		Host:  "ellyai.pro",
		Label: "EllyAI",
		Message: "WuProj категорически отказывается использовать API-ключ недобросовестного провайдера. " +
			"Просьба выбрать другого провайдера или использовать WuApi.",
	},
}

// isBlockedProvider returns the matching blocklist entry when
// `baseURL` resolves to a banned host, or (zero-value, false)
// otherwise. An unparseable URL is treated as not-blocked — the
// handler's earlier validation already rejected those.
func isBlockedProvider(baseURL string) (blockedProvider, bool) {
	if baseURL == "" {
		return blockedProvider{}, false
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return blockedProvider{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return blockedProvider{}, false
	}
	for _, b := range blockedProviders {
		bh := strings.ToLower(b.Host)
		if host == bh || strings.HasSuffix(host, "."+bh) {
			return b, true
		}
	}
	return blockedProvider{}, false
}
