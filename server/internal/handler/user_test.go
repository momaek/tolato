package handler

import (
	"testing"

	"github.com/momaek/tolato/server/internal/model"
)

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", model.RoleMember, true}, // omitted role defaults to the safer tier
		{model.RoleMember, model.RoleMember, true},
		{model.RoleAdmin, model.RoleAdmin, true},
		{"Admin", "", false}, // case-sensitive on purpose
		{"root", "", false},  // not a role in this system
		{"superuser", "", false},
	}

	for _, tt := range tests {
		got, ok := normalizeRole(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("normalizeRole(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

// An already-disabled admin isn't holding the instance open, so operating on
// one must not be blocked — and must not need a database round trip to decide.
func TestBlockedByLastAdminIgnoresInactiveAdmins(t *testing.T) {
	target := &model.User{Role: model.RoleAdmin, Status: model.UserStatusDisabled}
	if _, ok := blockedByLastAdmin(target); !ok {
		t.Error("a disabled admin was treated as the last active administrator")
	}
}
