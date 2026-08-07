package model

import "testing"

func TestLevelAtLeast(t *testing.T) {
	tests := []struct {
		have, want string
		ok         bool
	}{
		{LevelViewer, LevelViewer, true},
		{LevelOperator, LevelViewer, true},
		{LevelManager, LevelViewer, true},
		{LevelManager, LevelOperator, true},
		{LevelOperator, LevelManager, false},
		{LevelViewer, LevelOperator, false},
		// "no level" is not a weak level — it satisfies nothing, including the
		// weakest requirement. Treating "" as viewer would silently open every
		// read path to users with no grant at all.
		{"", LevelViewer, false},
		{"", "", false},
		{"root", LevelViewer, false},
		{LevelManager, "root", false},
	}

	for _, tt := range tests {
		if got := LevelAtLeast(tt.have, tt.want); got != tt.ok {
			t.Errorf("LevelAtLeast(%q, %q) = %v, want %v", tt.have, tt.want, got, tt.ok)
		}
	}
}

func TestHigherLevel(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{LevelViewer, LevelOperator, LevelOperator},
		{LevelManager, LevelViewer, LevelManager},
		{LevelOperator, LevelOperator, LevelOperator},
		{"", LevelViewer, LevelViewer},
		{LevelViewer, "", LevelViewer},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := HigherLevel(tt.a, tt.b); got != tt.want {
			t.Errorf("HigherLevel(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsValidLevel(t *testing.T) {
	for _, s := range []string{LevelViewer, LevelOperator, LevelManager} {
		if !IsValidLevel(s) {
			t.Errorf("IsValidLevel(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "admin", "owner", "Viewer"} {
		if IsValidLevel(s) {
			t.Errorf("IsValidLevel(%q) = true, want false", s)
		}
	}
}
