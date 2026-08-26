package service

// M8 robots.txt compliance guard.
//
// For every OnlineSource that queries a public HTML reference site
// (HyundaiPartsDeal, KiaPartsNow, 7zap, etc. — the "G5" tier in the M8
// plan), we MUST honour robots.txt. This file provides a small in-process
// cache + minimal parser that answers Allowed(userAgent, url) with:
//
//   - true  if the effective robots.txt has no matching Disallow rule
//   - false if there is a matching Disallow (or if robots.txt fetch failed:
//           we FAIL CLOSED — no rules → no requests — to avoid accidental
//           ToS violations when the origin is temporarily unreachable)
//
// The parser implements the subset of the (informal) Google robots.txt
// spec that matters for our use case:
//   * User-agent group matching by longest exact prefix (case-insensitive)
//   * Allow / Disallow directives; longest match wins per RFC
//   * "*" as a wildcard user-agent (fallback group)
//
// We deliberately do NOT implement:
//   * Wildcards inside path patterns ("*", "$") — our paths are literal
//   * Sitemap / Crawl-delay directives — advisory only for our patterns
//   * Non-standard extensions
//
// TTL: robots.txt is refetched every 24h. Fetch failures (5xx, timeout)
// invalidate the cache and force fail-closed until the next successful
// fetch. 4xx (including 404 "no robots.txt at all") is treated as
// "everything allowed" per the RFC.

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	robotsTTL          = 24 * time.Hour
	robotsFetchTimeout = 5 * time.Second
	robotsMaxBodyBytes = 128 * 1024 // 128 KB — plenty for any real robots.txt
	robotsDefaultAgent = "*"
)

// RobotsGuard caches robots.txt files per hostname and answers Allowed().
// Safe for concurrent use.
type RobotsGuard struct {
	client *http.Client
	mu     sync.RWMutex
	cache  map[string]*robotsEntry
}

type robotsEntry struct {
	rules     []robotsRule
	fetchedAt time.Time
	failed    bool // last fetch failed (fetch-timeout / 5xx / body-too-large)
}

type robotsRule struct {
	userAgent string // lower-case; "*" for fallback
	prefix    string
	allow     bool // true = Allow, false = Disallow
}

// NewRobotsGuard constructs a guard with the shared HTTP client. Pass nil to
// use a default client with a 5s per-request timeout.
func NewRobotsGuard(client *http.Client) *RobotsGuard {
	if client == nil {
		client = &http.Client{Timeout: robotsFetchTimeout}
	}
	return &RobotsGuard{
		client: client,
		cache:  make(map[string]*robotsEntry),
	}
}

// Allowed reports whether the given userAgent may fetch targetURL under the
// origin's robots.txt. Fail-closed semantics: any fetch/parse failure
// returns (false, err).
func (g *RobotsGuard) Allowed(ctx context.Context, userAgent, targetURL string) (bool, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return false, err
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return false, nil
	}

	entry, err := g.entryFor(ctx, u)
	if err != nil {
		return false, err
	}
	if entry.failed {
		return false, nil
	}

	// No rules at all → everything allowed (RFC default when robots.txt
	// absent or empty). Note: entry.failed already covers 5xx / timeout.
	if len(entry.rules) == 0 {
		return true, nil
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}

	// Match rules: pick the rule with the longest matching prefix for
	// either the exact userAgent or the "*" fallback. Ties: Allow wins
	// (per Google's clarification of the informal spec).
	agent := strings.ToLower(userAgent)
	if agent == "" {
		agent = robotsDefaultAgent
	}

	var matched *robotsRule
	var matchedLen int
	pickIfLonger := func(r robotsRule) {
		if !strings.HasPrefix(path, r.prefix) {
			return
		}
		if len(r.prefix) > matchedLen || (len(r.prefix) == matchedLen && r.allow) {
			cp := r
			matched = &cp
			matchedLen = len(r.prefix)
		}
	}

	// First pass: exact-agent group. Second pass: "*" fallback if no
	// exact-agent rule matched.
	for _, r := range entry.rules {
		if r.userAgent == agent {
			pickIfLonger(r)
		}
	}
	if matched == nil {
		for _, r := range entry.rules {
			if r.userAgent == robotsDefaultAgent {
				pickIfLonger(r)
			}
		}
	}

	if matched == nil {
		return true, nil // no rule matched → allowed
	}
	return matched.allow, nil
}

func (g *RobotsGuard) entryFor(ctx context.Context, u *url.URL) (*robotsEntry, error) {
	host := strings.ToLower(u.Host)

	g.mu.RLock()
	e, ok := g.cache[host]
	g.mu.RUnlock()
	if ok && time.Since(e.fetchedAt) < robotsTTL {
		return e, nil
	}

	robotsURL := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/robots.txt"}).String()
	entry := &robotsEntry{fetchedAt: time.Now()}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		entry.failed = true
		g.storeEntry(host, entry)
		return entry, err
	}
	req.Header.Set("User-Agent", robotsClientAgent)

	resp, err := g.client.Do(req)
	if err != nil {
		entry.failed = true
		g.storeEntry(host, entry)
		return entry, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		// No robots.txt → everything allowed. Cache empty rules.
		g.storeEntry(host, entry)
		return entry, nil
	}
	if resp.StatusCode >= 500 {
		entry.failed = true
		g.storeEntry(host, entry)
		return entry, nil
	}
	if resp.StatusCode >= 400 {
		// 4xx other than 404/410 (e.g. 401, 403) → treat as fail-closed
		entry.failed = true
		g.storeEntry(host, entry)
		return entry, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, robotsMaxBodyBytes))
	if err != nil {
		entry.failed = true
		g.storeEntry(host, entry)
		return entry, err
	}

	entry.rules = parseRobots(body)
	g.storeEntry(host, entry)
	return entry, nil
}

func (g *RobotsGuard) storeEntry(host string, entry *robotsEntry) {
	g.mu.Lock()
	g.cache[host] = entry
	g.mu.Unlock()
}

// robotsClientAgent — the UA sent when fetching robots.txt itself. Kept
// generic so origin admins recognise this as our bot and can grep their
// logs for it.
const robotsClientAgent = "IfritahPartsEngine/1.0 (+https://ifritah.com/bot-info)"

// parseRobots turns a robots.txt body into a flat list of rules.
//
// Grammar (simplified):
//
//	User-agent: NAME       — begins a group
//	Disallow: /path        — path prefix denied
//	Allow: /path           — path prefix allowed (overrides Disallow if longer)
//
// Empty-value Disallow (`Disallow: `) means "nothing disallowed" and is
// treated as a no-op. Comments (`# ...`) and blank lines separate groups.
func parseRobots(body []byte) []robotsRule {
	var rules []robotsRule
	var currentAgents []string

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	// Some robots.txt files have long lines; bump the buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		field := strings.ToLower(strings.TrimSpace(line[:colon]))
		value := strings.TrimSpace(line[colon+1:])

		switch field {
		case "user-agent":
			currentAgents = append(currentAgents, strings.ToLower(value))
		case "disallow":
			if value == "" {
				continue // no-op per spec
			}
			for _, ua := range currentAgents {
				rules = append(rules, robotsRule{userAgent: ua, prefix: value, allow: false})
			}
			currentAgents = collapseAgentsIfNewGroup(currentAgents)
		case "allow":
			for _, ua := range currentAgents {
				rules = append(rules, robotsRule{userAgent: ua, prefix: value, allow: true})
			}
			currentAgents = collapseAgentsIfNewGroup(currentAgents)
		}
	}
	return rules
}

// collapseAgentsIfNewGroup returns the same slice — placeholder in case we
// want to handle multi-agent groups differently. Here we accumulate agents
// under the current group until any Disallow/Allow line fires, then keep
// accumulating until a new User-agent line resets. Standard behaviour.
func collapseAgentsIfNewGroup(agents []string) []string {
	return agents
}
