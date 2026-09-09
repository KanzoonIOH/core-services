package handler

import (
	"testing"

	"github.com/google/uuid"
)

func TestCanViewConversation(t *testing.T) {
	me := uuid.New()
	someoneElse := uuid.New()

	cases := []struct {
		name  string
		owner *uuid.UUID
		role  string
		want  bool
	}{
		{"own conversation", &me, "VIEWER", true},
		{"someone else's", &someoneElse, "VIEWER", false},
		{"someone else's, admin", &someoneElse, "ADMIN", true},
		{"someone else's, superadmin", &someoneElse, "SUPERADMIN", true},
		{"ownerless (api key traffic)", nil, "VIEWER", true},
	}

	for _, c := range cases {
		if got := canViewConversation(c.owner, me, c.role); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
