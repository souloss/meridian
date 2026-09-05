package service

import (
	"testing"
	"uuid"
)

func TestParseRevisionETag(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("018f0b7a-3b54-7c2e-b7ef-3baf9709a2a1")
	revision, err := parseRevisionETag(`"credential:`+id.String()+`:7"`, "credential", id)
	if err != nil || revision != 7 {
		t.Fatalf("parseRevisionETag() = %d, %v", revision, err)
	}
	for _, value := range []string{
		`credential:` + id.String() + `:7"`,
		`"credential:` + id.String() + `:0"`,
		`"global-credential:` + id.String() + `:7"`,
	} {
		if _, err := parseRevisionETag(value, "credential", id); err == nil {
			t.Errorf("parseRevisionETag(%q) unexpectedly succeeded", value)
		}
	}
}

func TestNormalizeKnownHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "dns", input: " Git.Example.COM ", want: "git.example.com"},
		{name: "bracketed ipv6", input: "[2001:DB8::1]", want: "2001:db8::1"},
		{name: "ipv4", input: "192.0.2.10", want: "192.0.2.10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeKnownHost(test.input)
			if err != nil || got != test.want {
				t.Fatalf("normalizeKnownHost(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
	for _, input := range []string{"", "-bad.example", "bad..example", "bad/example", "bad.example."} {
		if _, err := normalizeKnownHost(input); err == nil {
			t.Errorf("normalizeKnownHost(%q) unexpectedly succeeded", input)
		}
	}
}

func TestCredentialSharingValidation(t *testing.T) {
	t.Parallel()

	teamID := uuid.MustParse("018f0b7a-3b54-7c2e-b7ef-3baf9709a2a1")
	if err := validateSharing("private", nil); err != nil {
		t.Fatal(err)
	}
	if err := validateSharing("team", []uuid.UUID{teamID}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		scope string
		ids   []uuid.UUID
	}{
		{scope: "private", ids: []uuid.UUID{teamID}},
		{scope: "team"},
		{scope: "team", ids: []uuid.UUID{teamID, teamID}},
		{scope: "unknown"},
	} {
		if err := validateSharing(test.scope, test.ids); err == nil {
			t.Errorf("validateSharing(%q, %v) unexpectedly succeeded", test.scope, test.ids)
		}
	}
}
