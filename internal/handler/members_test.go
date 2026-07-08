package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"testing"

	"github.com/google/uuid"
)

func TestCanEditMember(t *testing.T) {
	admin := uuid.New()
	super := uuid.New()
	other := uuid.New()

	cases := []struct {
		name       string
		actorID    uuid.UUID
		actorRole  string
		targetID   uuid.UUID
		targetRole db.UserRole
		want       bool
	}{
		{"admin edits viewer", admin, "ADMIN", other, db.UserRoleVIEWER, true},
		{"admin edits self", admin, "ADMIN", admin, db.UserRoleADMIN, false},
		{"admin edits other admin", admin, "ADMIN", other, db.UserRoleADMIN, false},
		{"admin edits superadmin", admin, "ADMIN", other, db.UserRoleSUPERADMIN, false},
		{"super edits admin", super, "SUPERADMIN", other, db.UserRoleADMIN, true},
		{"super edits self", super, "SUPERADMIN", super, db.UserRoleSUPERADMIN, false},
		{"super edits other super", super, "SUPERADMIN", other, db.UserRoleSUPERADMIN, false},
		{"super edits viewer", super, "SUPERADMIN", other, db.UserRoleVIEWER, true},
	}

	for _, c := range cases {
		got := canEditMember(c.actorID, c.actorRole, c.targetID, c.targetRole)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
