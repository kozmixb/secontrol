package app

import "testing"

func TestParseSemanticTag(t *testing.T) {
	tests := []struct {
		tag       string
		want      semanticVersion
		wantValid bool
	}{
		{tag: "1.1.15", want: semanticVersion{1, 1, 15}, wantValid: true},
		{tag: "v2.4.0", want: semanticVersion{2, 4, 0}, wantValid: true},
		{tag: "1.8", want: semanticVersion{1, 8, 0}, wantValid: true},
		{tag: "latest"},
		{tag: "1.2.3-alpine"},
		{tag: "1"},
	}
	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			got, valid := parseSemanticTag(test.tag)
			if valid != test.wantValid || got != test.want {
				t.Fatalf("parseSemanticTag(%q) = %#v, %v; want %#v, %v", test.tag, got, valid, test.want, test.wantValid)
			}
		})
	}
}

func TestCompareSemanticVersion(t *testing.T) {
	if compareSemanticVersion(semanticVersion{1, 1, 16}, semanticVersion{1, 1, 15}) <= 0 {
		t.Fatal("expected patch update to be newer")
	}
	if compareSemanticVersion(semanticVersion{2, 0, 0}, semanticVersion{1, 99, 99}) <= 0 {
		t.Fatal("expected major update to be newer")
	}
	if compareSemanticVersion(semanticVersion{1, 1, 15}, semanticVersion{1, 1, 15}) != 0 {
		t.Fatal("expected identical versions to compare equally")
	}
}

func TestImageWithTag(t *testing.T) {
	tests := map[string]string{
		"owner/app:1.1.15":                       "owner/app:1.2.0",
		"registry.example:5000/owner/app:1.1.15": "registry.example:5000/owner/app:1.2.0",
		"owner/app@sha256:abc":                   "owner/app:1.2.0",
	}
	for image, want := range tests {
		if got := imageWithTag(image, "1.2.0"); got != want {
			t.Errorf("imageWithTag(%q) = %q; want %q", image, got, want)
		}
	}
}
