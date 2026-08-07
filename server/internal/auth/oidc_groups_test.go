package auth

import (
	"encoding/json"
	"testing"
)

// IdPs disagree on the shape of a groups claim, so all the plausible ones have
// to decode rather than failing the whole login.
func TestParseGroupsClaim(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"array of names", `["ops","dev"]`, []string{"ops", "dev"}, false},
		{"empty array", `[]`, nil, false},
		{"single bare string", `"ops"`, []string{"ops"}, false},
		{"empty string", `""`, nil, false},
		{"null", `null`, nil, false},
		{"a number is not a group", `42`, nil, true},
		{"an object is not a group list", `{"a":1}`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGroupsClaim(json.RawMessage(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGroupsClaim(%s) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseGroupsClaim(%s) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseGroupsClaim(%s)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// An empty array and a missing claim look similar but mean opposite things.
// Conflating them is how a mistyped claim name silently strips everyone's
// access on their next sign-in, so the distinction is asserted here.
func TestGroupsPresentDistinguishesEmptyFromAbsent(t *testing.T) {
	empty, err := parseGroupsClaim(json.RawMessage(`[]`))
	if err != nil {
		t.Fatalf("parseGroupsClaim: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("an empty claim array should decode to no groups, got %v", empty)
	}

	// The absent case never reaches parseGroupsClaim at all: Exchange only
	// calls it when the key exists, leaving GroupsPresent false.
	var absent OIDCIdentity
	if absent.GroupsPresent {
		t.Error("GroupsPresent must default to false so an absent claim is not read as 'no groups'")
	}
}
