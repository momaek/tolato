package handler

import "testing"

func TestHasUpdate(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.8.9", "v0.8.10", true},  // patch bump
		{"v0.8.10", "v0.8.9", false}, // older latest (10 > 9 numerically, not lexically)
		{"v0.8.10", "v0.8.10", false},
		{"v0.8.10", "v0.9.0", true}, // minor bump
		{"v0.9.0", "v1.0.0", true},  // major bump
		{"v1.0.0", "v0.9.9", false},
		{"v0.8.10-3-gdeadbee", "v0.8.10", false}, // dirty/describe current, same release
		{"v0.8.10-3-gdeadbee", "v0.8.11", true},  // dirty current, newer release
		{"dev", "v0.8.10", false},                // unparseable current => no false dot
		{"v0.8.10", "", false},                   // latest unresolved => no dot
		{"", "", false},
	}
	for _, c := range cases {
		if got := hasUpdate(c.current, c.latest); got != c.want {
			t.Errorf("hasUpdate(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestSemverExtraction(t *testing.T) {
	// resolveLatestTag pulls a tag out of a redirect Location like
	// .../releases/tag/v0.8.10 — verify the regex isolates it.
	got := semverRe.FindString("https://github.com/momaek/tolato/releases/tag/v0.8.10")
	if got != "v0.8.10" {
		t.Errorf("semver extraction = %q, want v0.8.10", got)
	}
}
