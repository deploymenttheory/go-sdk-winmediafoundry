package esd

import "testing"

func buildSampleCatalog() *ESDCatalog {
	return &ESDCatalog{Images: []ESDImage{
		{FileName: "26100.4349.250607-1500.ge_release_CLIENTCONSUMER_RET_A64FRE_en-us.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "en-us"},
		{FileName: "26100.4349.250607-1500.ge_release_CLIENTCONSUMER_RET_x64FRE_en-us.esd", Edition: "Professional", Architecture: "x64", LanguageCode: "en-us"},
		{FileName: "26200.5001.250901-1200.ge_release_CLIENTCONSUMER_RET_A64FRE_en-us.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "en-us"},
		{FileName: "garbage-no-build.esd", Edition: "Core", Architecture: "ARM64", LanguageCode: "en-us"},
	}}
}

func TestBuildMajor(t *testing.T) {
	tests := []struct {
		file string
		want int
	}{
		{"26100.4349.250607-1500.ge_release.esd", 26100},
		{"26200.5001.x.esd", 26200},
		{"garbage-no-build.esd", 0},
		{".leadingdot", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := (ESDImage{FileName: tt.file}).BuildMajor(); got != tt.want {
			t.Errorf("BuildMajor(%q) = %d, want %d", tt.file, got, tt.want)
		}
	}
}

func TestFilterBuildMajor(t *testing.T) {
	c := buildSampleCatalog()

	arm24 := c.FilterBuildMajor(26100, "Professional", "ARM64", "en-us")
	if len(arm24) != 1 || arm24[0].BuildMajor() != 26100 {
		t.Fatalf("FilterBuildMajor(26100, ARM64) = %+v, want exactly the 26100 ARM64 image", arm24)
	}

	arm25 := c.FilterBuildMajor(26200, "Professional", "ARM64", "en-us")
	if len(arm25) != 1 || arm25[0].BuildMajor() != 26200 {
		t.Fatalf("FilterBuildMajor(26200, ARM64) = %+v, want exactly the 26200 ARM64 image", arm25)
	}

	// A build absent for the requested arch returns nothing.
	if got := c.FilterBuildMajor(26200, "Professional", "x64", "en-us"); len(got) != 0 {
		t.Fatalf("FilterBuildMajor(26200, x64) = %+v, want empty", got)
	}

	// build 0 disables the build filter.
	if got := c.FilterBuildMajor(0, "", "ARM64", "en-us"); len(got) != 3 {
		t.Fatalf("FilterBuildMajor(0, ARM64) returned %d images, want 3", len(got))
	}
}

func TestBuildMajors(t *testing.T) {
	got := buildSampleCatalog().BuildMajors()
	// 26100, 26200, and 0 (the unparseable filename).
	want := map[int]bool{26100: true, 26200: true, 0: true}
	if len(got) != len(want) {
		t.Fatalf("BuildMajors() = %v, want keys %v", got, want)
	}
	for _, b := range got {
		if !want[b] {
			t.Errorf("BuildMajors() returned unexpected %d", b)
		}
	}
}
