package middleware

import (
	"net/http"
	"testing"
)

func TestActionFor(t *testing.T) {
	cases := map[string]string{
		http.MethodPost:   "CREATE",
		http.MethodPatch:  "UPDATE",
		http.MethodPut:    "UPDATE",
		http.MethodDelete: "DELETE",
		http.MethodGet:    "", // reads are not audited
		http.MethodOptions: "",
	}
	for method, want := range cases {
		if got := actionFor(method); got != want {
			t.Errorf("actionFor(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestMenuFor(t *testing.T) {
	cases := map[string]string{
		"/api/agents":            "agents",
		"/api/agents/123":        "agents",
		"/api/agents/123/persona": "agents",
		"/api/knowledges/abc":    "knowledges",
		"/api/members/x/status":  "members",
		"/api/connect/agent-mcp": "connect",
		"/api/global-config/":    "global-config",
	}
	for path, want := range cases {
		if got := menuFor(path); got != want {
			t.Errorf("menuFor(%q) = %q, want %q", path, got, want)
		}
	}
}
