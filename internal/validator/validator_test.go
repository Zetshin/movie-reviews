package validator

import "testing"

func TestValidator(t *testing.T) {
	var v Validator
	if !v.Valid() {
		t.Fatal("empty validator should be valid")
	}
	v.CheckField(false, "name", "first")
	v.CheckField(false, "name", "second")
	if v.Valid() || v.FieldErrors["name"] != "first" {
		t.Fatalf("first error should be kept, got %+v", v.FieldErrors)
	}

	var n Validator
	n.AddNonFieldError("bad")
	if n.Valid() {
		t.Fatal("non-field errors should make the form invalid")
	}
}

func TestHelpers(t *testing.T) {
	if NotBlank("  \t") || !NotBlank(" a ") {
		t.Error("NotBlank")
	}
	// Counts runes, not bytes, so Thai text is measured correctly.
	if !MaxChars("สวัสดี", 6) || MaxChars("สวัสดี", 5) {
		t.Error("MaxChars should count characters")
	}
	if !MinChars("abcdefgh", 8) || MinChars("abc", 8) {
		t.Error("MinChars")
	}
	if !MaxBytes("ab", 2) || MaxBytes("สวัสดี", 10) {
		t.Error("MaxBytes should count bytes")
	}
	if !PermittedValue(2.5, 0.5, 2.5, 5.0) || PermittedValue(2.4, 0.5, 2.5, 5.0) {
		t.Error("PermittedValue")
	}
}

func TestEmailRX(t *testing.T) {
	for email, want := range map[string]bool{
		"user@example.com":   true,
		"a.b+tag@sub.ex.org": true,
		"no-at-sign":         false,
		"user@":              false,
		"@example.com":       false,
		"user@exa mple.com":  false,
	} {
		if got := Matches(email, EmailRX); got != want {
			t.Errorf("%q: got %v, want %v", email, got, want)
		}
	}
}
