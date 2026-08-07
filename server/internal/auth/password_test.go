package auth

import "testing"

func TestCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !CheckPassword(hash, "correct horse battery") {
		t.Error("the correct password was rejected")
	}
	if CheckPassword(hash, "correct horse batter") {
		t.Error("a wrong password was accepted")
	}
	if CheckPassword(hash, "") {
		t.Error("an empty password was accepted")
	}
}

// An account with no local password (OIDC-only, or a half-written row) must
// never authenticate — including against the empty string, which is what a
// naive equality check would let through.
func TestCheckPasswordRejectsEmptyHash(t *testing.T) {
	for _, attempt := range []string{"", "anything"} {
		if CheckPassword("", attempt) {
			t.Errorf("empty hash accepted password %q", attempt)
		}
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	a, err := HashPassword("same input")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same input")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Error("identical passwords produced identical hashes, so the salt is not random")
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		pass    string
		wantErr bool
	}{
		{"empty", "", true},
		{"one short", "1234567", true},
		{"exactly the minimum", "12345678", false},
		{"long", "a much longer passphrase", false},
		// Length is counted in runes, not bytes: eight Chinese characters are
		// 24 bytes but still exactly eight characters to the person typing them.
		{"eight multibyte runes", "密码密码密码密码", false},
		{"seven multibyte runes", "密码密码密码密", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pass)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.pass, err, tt.wantErr)
			}
		})
	}
}
