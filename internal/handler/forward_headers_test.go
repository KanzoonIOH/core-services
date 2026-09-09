package handler

import (
	"net/http"
	"testing"
)

// A forwarded Accept-Encoding makes Go's transport skip auto-decompression, so
// the upstream JSON comes back br/zstd-encoded and the assistant turn never
// persists. It must never reach the upstream request.
func TestCopyForwardHeadersDropsAcceptEncoding(t *testing.T) {
	src := http.Header{
		"Accept-Encoding": {"gzip, deflate, br, zstd"},
		"Authorization":   {"Bearer caller"},
		"Connection":      {"keep-alive"},
		"X-Trace-Id":      {"abc"},
	}
	dst := http.Header{}
	copyForwardHeaders(dst, src)

	if got := dst.Get("Accept-Encoding"); got != "" {
		t.Errorf("Accept-Encoding forwarded = %q, want dropped", got)
	}
	if got := dst.Get("X-Trace-Id"); got != "abc" {
		t.Errorf("X-Trace-Id = %q, want %q", got, "abc")
	}
}
