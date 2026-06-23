package constants

import "testing"

func TestReleaseBuild(t *testing.T) {
	tests := []struct {
		in    Release
		want  int
		known bool
	}{
		{Release24H2, 26100, true},
		{Release25H2, 26200, true},
		{Release23H2, 22631, true},
		{"24h2", 26100, true}, // case-insensitive
		{"25H2", 26200, true},
		{"99H9", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := ReleaseBuild(tt.in)
		if ok != tt.known || got != tt.want {
			t.Errorf("ReleaseBuild(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.known)
		}
	}
}

func TestReleases(t *testing.T) {
	got := Releases()
	if len(got) == 0 {
		t.Fatal("Releases() returned no entries")
	}
	for _, r := range got {
		if _, ok := ReleaseBuild(r); !ok {
			t.Errorf("Releases() returned %q which ReleaseBuild does not know", r)
		}
	}
}
