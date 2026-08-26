package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRobots_DisallowSection(t *testing.T) {
	body := []byte(`
User-agent: *
Disallow: /admin/
Disallow: /private
Allow: /public

User-agent: BadBot
Disallow: /
`)
	rules := parseRobots(body)
	if len(rules) < 4 {
		t.Fatalf("expected ≥4 rules, got %d: %+v", len(rules), rules)
	}
	// Sanity: at least one Disallow for user-agent "*" with prefix "/admin/"
	var found bool
	for _, r := range rules {
		if r.userAgent == "*" && r.prefix == "/admin/" && !r.allow {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '* Disallow /admin/' rule, not present")
	}
}

func TestRobotsGuard_AllowedWhenNoRules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r) // no robots.txt
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	g := NewRobotsGuard(nil)
	ok, err := g.Allowed(context.Background(), "IfritahPartsEngine/1.0", srv.URL+"/some/path")
	if err != nil {
		t.Fatalf("Allowed err: %v", err)
	}
	if !ok {
		t.Errorf("expected allowed when robots.txt 404")
	}
}

func TestRobotsGuard_DisallowMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`User-agent: *
Disallow: /admin/
Allow: /admin/public/
`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	g := NewRobotsGuard(nil)
	// /admin/secret → Disallow
	ok, err := g.Allowed(context.Background(), "IfritahPartsEngine/1.0", srv.URL+"/admin/secret")
	if err != nil {
		t.Fatalf("Allowed err: %v", err)
	}
	if ok {
		t.Errorf("expected disallow for /admin/secret")
	}
	// /admin/public/x → Allow (more-specific)
	ok, err = g.Allowed(context.Background(), "IfritahPartsEngine/1.0", srv.URL+"/admin/public/x")
	if err != nil {
		t.Fatalf("Allowed err: %v", err)
	}
	if !ok {
		t.Errorf("expected allow for /admin/public/x (more-specific)")
	}
}

func TestRobotsGuard_FailClosedOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
	}))
	defer srv.Close()

	g := NewRobotsGuard(nil)
	ok, err := g.Allowed(context.Background(), "IfritahPartsEngine/1.0", srv.URL+"/anything")
	if err != nil {
		// entryFor returns nil error but entry.failed=true → Allowed
		// returns (false, nil). Either shape is acceptable — the guard
		// contract is only "fail closed".
		_ = err
	}
	if ok {
		t.Errorf("expected fail-closed on 5xx robots.txt")
	}
}

func TestRobotsGuard_EmptyRulesAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`# just a comment
`))
			return
		}
	}))
	defer srv.Close()

	g := NewRobotsGuard(nil)
	ok, err := g.Allowed(context.Background(), "IfritahPartsEngine/1.0", srv.URL+"/anywhere")
	if err != nil {
		t.Fatalf("Allowed err: %v", err)
	}
	if !ok {
		t.Errorf("expected allowed when robots.txt has only comments")
	}
}
