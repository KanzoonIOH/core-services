package handler

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestIPAllowed(t *testing.T) {
	allow := []netip.Addr{netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("10.0.0.1")}
	if !ipAllowed(netip.MustParseAddr("203.0.113.5"), allow) {
		t.Fatal("listed ip should be allowed")
	}
	if ipAllowed(netip.MustParseAddr("203.0.113.6"), allow) {
		t.Fatal("unlisted ip should be denied")
	}
}

func TestClientIP(t *testing.T) {
	r := &http.Request{Header: http.Header{}, RemoteAddr: "192.0.2.1:5555"}
	if got := clientIP(r); got != netip.MustParseAddr("192.0.2.1") {
		t.Fatalf("RemoteAddr: got %v", got)
	}

	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := clientIP(r); got != netip.MustParseAddr("203.0.113.5") {
		t.Fatalf("XFF first hop: got %v", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	allow := []string{"https://app.example.com", "https://widget.example.com"}
	if !originAllowed("https://app.example.com", allow) {
		t.Fatal("listed origin should be allowed")
	}
	if originAllowed("https://evil.example.com", allow) {
		t.Fatal("unlisted origin should be denied")
	}
	// Empty list = allow all is enforced by the caller, not this helper.
	if originAllowed("https://app.example.com", nil) {
		t.Fatal("empty list matches nothing in the helper")
	}
}
