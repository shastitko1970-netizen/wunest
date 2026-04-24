// Package outboundproxy manages a pool of HTTP proxies that outbound BYOK
// calls route through. Needed because our server IP (Selectel / Russia) is
// geo-blocked by OpenAI and Anthropic — without a proxy every direct-
// provider call 403s with "unsupported_country_region_territory".
//
// Input format is deliberately simple: `host:port:user:pass` per line,
// comma- or newline-separated, so the same list you'd paste into any
// other HTTP client works here too.
//
// The Transport round-robins across healthy proxies per-request. Failed
// proxies are marked briefly-dead (60s cooldown) so repeated retries
// during an outage don't thrash the same dead box.
package outboundproxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Pool is a thread-safe rotation of HTTP proxies with per-proxy "dead"
// cooldowns. A nil or zero-length pool is legal: Transport() returns the
// default transport unchanged, so callers don't need to branch.
type Pool struct {
	mu      sync.Mutex
	proxies []*entry
	next    int
}

type entry struct {
	url      *url.URL
	deadTill time.Time
}

const deadCooldown = 60 * time.Second

// Parse accepts a comma- OR newline-separated list of `host:port:user:pass`
// tuples. Blank lines and lines starting with `#` are skipped.
//
// Returns nil, nil when spec is empty — a caller that didn't configure the
// env var shouldn't be forced to handle an error just to say "no proxy".
func Parse(spec string) (*Pool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	// Normalise separators — accept either commas or newlines (most password
	// manager exports use one per line; env vars are easier as a CSV).
	norm := strings.ReplaceAll(spec, "\n", ",")
	norm = strings.ReplaceAll(norm, "\r", ",")
	parts := strings.Split(norm, ",")
	out := make([]*entry, 0, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		u, err := parseLine(raw)
		if err != nil {
			return nil, fmt.Errorf("outboundproxy: parse %q: %w", maskLine(raw), err)
		}
		out = append(out, &entry{url: u})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &Pool{proxies: out}, nil
}

// parseLine turns `host:port:user:pass` into an http://user:pass@host:port/ URL.
// Also accepts a fully-qualified URL (http://..., socks5://...) so custom
// list formats don't require editing the lib.
func parseLine(raw string) (*url.URL, error) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		return u, nil
	}
	fields := strings.Split(raw, ":")
	switch len(fields) {
	case 2: // host:port (no auth)
		return url.Parse("http://" + fields[0] + ":" + fields[1])
	case 4: // host:port:user:pass
		host, port, user, pass := fields[0], fields[1], fields[2], fields[3]
		u := &url.URL{
			Scheme: "http",
			User:   url.UserPassword(user, pass),
			Host:   host + ":" + port,
		}
		return u, nil
	default:
		return nil, fmt.Errorf("expected host:port or host:port:user:pass, got %d fields", len(fields))
	}
}

// maskLine hides the password in a parsing-error message so a bad entry in
// logs doesn't leak credentials.
func maskLine(raw string) string {
	fields := strings.Split(raw, ":")
	if len(fields) >= 4 {
		return fields[0] + ":" + fields[1] + ":" + fields[2] + ":***"
	}
	return raw
}

// Size returns how many proxies the pool holds (live + dead).
func (p *Pool) Size() int {
	if p == nil {
		return 0
	}
	return len(p.proxies)
}

// pick picks the next live proxy in round-robin order. Returns nil if every
// proxy is currently in a dead-cooldown. When that happens, callers should
// fall through to a direct connection — a dead pool is still better than
// refusing the request.
func (p *Pool) pick() *entry {
	if p == nil || len(p.proxies) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for i := 0; i < len(p.proxies); i++ {
		idx := (p.next + i) % len(p.proxies)
		e := p.proxies[idx]
		if e.deadTill.IsZero() || now.After(e.deadTill) {
			// Advance the cursor so the NEXT call tries the next proxy.
			p.next = (idx + 1) % len(p.proxies)
			return e
		}
	}
	// Every proxy is cooling down. Reset the most-stale one so we keep
	// probing rather than hard-failing indefinitely.
	oldest := p.proxies[0]
	for _, e := range p.proxies[1:] {
		if e.deadTill.Before(oldest.deadTill) {
			oldest = e
		}
	}
	oldest.deadTill = time.Time{}
	return oldest
}

// markDead sets a cooldown on the proxy so the next Transport.Proxy call
// skips it. Called by the transport when it sees a dial error.
func (p *Pool) markDead(e *entry) {
	if p == nil || e == nil {
		return
	}
	p.mu.Lock()
	e.deadTill = time.Now().Add(deadCooldown)
	p.mu.Unlock()
}

// Transport returns an http.RoundTripper that routes every request through
// one of the pool's proxies. Failed proxies are marked dead and skipped on
// the next retry.
//
// On a nil/empty pool, returns http.DefaultTransport — so calling code can
// always do: `client := &http.Client{Transport: pool.Transport()}` without
// a nil check.
func (p *Pool) Transport() http.RoundTripper {
	if p == nil || len(p.proxies) == 0 {
		return http.DefaultTransport
	}
	// Underlying transport does the TCP + TLS; we just swap Proxy per-call.
	// Cloning DefaultTransport keeps its sensible defaults (pool sizes,
	// timeouts, HTTP/2 support).
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = func(req *http.Request) (*url.URL, error) {
		e := p.pick()
		if e == nil {
			return nil, nil // no live proxy — fall through direct
		}
		// Stash the entry so ErrorHandler / next call can mark it dead if the
		// dial fails. Go's http.Transport doesn't expose the chosen proxy in
		// error callbacks, so we rely on the retrying-caller pattern: if the
		// RoundTrip returns a net/dial error, the caller calls MarkDead with
		// the last proxy (tracked via request header).
		req.Header.Set("X-Wunest-Proxy-Host", e.url.Host)
		return e.url, nil
	}
	return &retryingTransport{base: base, pool: p}
}

// retryingTransport wraps a Transport so a dial error against the chosen
// proxy pops it from the rotation and retries against the next one.
type retryingTransport struct {
	base *http.Transport
	pool *Pool
}

func (t *retryingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// At most len(proxies)+1 attempts: one per proxy, plus a final direct
	// attempt if everyone's dead. Loop caps protect against a config with
	// every proxy bad.
	maxAttempts := t.pool.Size() + 1
	if maxAttempts > 6 {
		maxAttempts = 6
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Clone the request each time — a read body from a previous attempt
		// would be drained. For GET /models / POST /chat/completions our
		// bodies are either nil or small, so we just rewind via GetBody.
		r := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			r.Body = body
		}
		resp, err := t.base.RoundTrip(r)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Only cycle to the next proxy on connection-level failures (the
		// proxy itself is unreachable). 4xx/5xx from the UPSTREAM come back
		// with err == nil and resp != nil — we don't retry those; they're
		// the caller's problem.
		if !isProxyDialErr(err) {
			return nil, err
		}
		host := req.Header.Get("X-Wunest-Proxy-Host")
		for _, e := range t.pool.proxies {
			if e.url.Host == host {
				t.pool.markDead(e)
				break
			}
		}
	}
	return nil, fmt.Errorf("all proxies failed: %w", lastErr)
}

// isProxyDialErr reports whether an http.Transport error looks like "couldn't
// reach the proxy" as opposed to "upstream returned HTTP 5xx". Conservative:
// only retries on explicit proxy-connect failures.
func isProxyDialErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "proxyconnect") ||
		strings.Contains(s, "connect: connection refused") ||
		strings.Contains(s, "connect: connection reset") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "no route to host")
}
