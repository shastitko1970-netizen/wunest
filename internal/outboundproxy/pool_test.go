package outboundproxy

import (
	"testing"
)

func TestParse_Empty(t *testing.T) {
	p, err := Parse("")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if p != nil {
		t.Fatal("empty spec should return nil pool, not an empty one")
	}
	// And nil pool's Transport is http.DefaultTransport (no panic).
	if tr := p.Transport(); tr == nil {
		t.Fatal("nil pool Transport() should be non-nil (DefaultTransport)")
	}
}

func TestParse_HostPortUserPass(t *testing.T) {
	p, err := Parse("45.150.63.89:62692:alice:secret")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Size() != 1 {
		t.Fatalf("want 1 proxy, got %d", p.Size())
	}
	u := p.proxies[0].url
	if u.Scheme != "http" {
		t.Errorf("scheme: want http, got %s", u.Scheme)
	}
	if u.Host != "45.150.63.89:62692" {
		t.Errorf("host: got %s", u.Host)
	}
	pw, ok := u.User.Password()
	if !ok || pw != "secret" {
		t.Errorf("password not set correctly")
	}
	if u.User.Username() != "alice" {
		t.Errorf("username: got %s", u.User.Username())
	}
}

func TestParse_HostPort_NoAuth(t *testing.T) {
	p, err := Parse("proxy.example.com:8080")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.proxies[0].url.User != nil {
		t.Error("no-auth proxy should have nil User")
	}
}

func TestParse_FullURL(t *testing.T) {
	p, err := Parse("socks5://user:pass@host.example:1080")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.proxies[0].url.Scheme != "socks5" {
		t.Errorf("full URL scheme dropped: %s", p.proxies[0].url.Scheme)
	}
}

func TestParse_MultipleSeparators(t *testing.T) {
	spec := `
# a comment
45.150.63.89:62692:u1:p1
92.119.161.152:63786:u2:p2

proxy.example.com:8080
`
	p, err := Parse(spec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Size() != 3 {
		t.Fatalf("expected 3 proxies, got %d", p.Size())
	}
}

func TestParse_CommaSeparated(t *testing.T) {
	p, err := Parse("host1:80:u:p,host2:81:u:p,host3:82:u:p")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Size() != 3 {
		t.Fatalf("expected 3 proxies, got %d", p.Size())
	}
}

func TestParse_BadLine(t *testing.T) {
	// Three fields (host:port:user) isn't a valid shape — we want a clear
	// error, not silent acceptance.
	_, err := Parse("host:80:onlyuser")
	if err == nil {
		t.Fatal("expected parse error for 3-field line")
	}
}

func TestPick_RoundRobin(t *testing.T) {
	p, err := Parse("a.example:80:u:p,b.example:80:u:p,c.example:80:u:p")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	hosts := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		e := p.pick()
		hosts = append(hosts, e.url.Host)
	}
	// Round-robin should cycle a → b → c → a → b → c.
	want := []string{"a.example:80", "b.example:80", "c.example:80", "a.example:80", "b.example:80", "c.example:80"}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("position %d: want %s, got %s", i, want[i], hosts[i])
		}
	}
}

func TestPick_SkipsDead(t *testing.T) {
	p, err := Parse("a.example:80,b.example:80,c.example:80")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Mark 'a' dead. Next pick should go to 'b'.
	p.markDead(p.proxies[0])
	e := p.pick()
	if e.url.Host != "b.example:80" {
		t.Errorf("after markDead a: expected b, got %s", e.url.Host)
	}
}

func TestPick_AllDead_Resurrects(t *testing.T) {
	p, err := Parse("a.example:80,b.example:80")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p.markDead(p.proxies[0])
	p.markDead(p.proxies[1])
	// All dead — pick should resurrect one (the stalest) so requests keep
	// trying rather than hard-failing.
	e := p.pick()
	if e == nil {
		t.Fatal("all-dead pool should still return a proxy (resurrected)")
	}
}

func TestMaskLine(t *testing.T) {
	got := maskLine("host:80:alice:secret")
	want := "host:80:alice:***"
	if got != want {
		t.Errorf("maskLine: got %q want %q", got, want)
	}
}
