package esd

import "testing"

func TestIsARM64(t *testing.T) {
	tests := []struct {
		name string
		img  ESDImage
		want bool
	}{
		{
			name: "arm64 by token and field",
			img:  ESDImage{FileName: "26100.4349.ge_release_CLIENTCONSUMER_RET_A64FRE_en-us.esd", Architecture: "ARM64"},
			want: true,
		},
		{
			name: "x64 by token and field",
			img:  ESDImage{FileName: "26100.4349.ge_release_CLIENTCONSUMER_RET_x64FRE_en-us.esd", Architecture: "x64"},
			want: false,
		},
		{
			name: "mislabeled x64 filename but ARM64 field is rejected",
			img:  ESDImage{FileName: "26100.4349.ge_release_CLIENTCONSUMER_RET_x64FRE_en-us.esd", Architecture: "ARM64"},
			want: false,
		},
		{
			name: "no token falls back to field",
			img:  ESDImage{FileName: "garbage-no-build.esd", Architecture: "ARM64"},
			want: true,
		},
		{
			name: "no token x64 field",
			img:  ESDImage{FileName: "garbage-no-build.esd", Architecture: "x64"},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.img.IsARM64(); got != tc.want {
				t.Fatalf("IsARM64() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterARM64BuildMajor(t *testing.T) {
	cat := &ESDCatalog{Images: []ESDImage{
		{FileName: "26100.1.ge_release_CLIENTCONSUMER_RET_A64FRE_en-us.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "en-us"},
		{FileName: "26100.1.ge_release_CLIENTCONSUMER_RET_x64FRE_en-us.esd", Edition: "Professional", Architecture: "x64", LanguageCode: "en-us"},
		// Mislabeled: claims ARM64 but the filename is x64 — must be dropped.
		{FileName: "26100.1.ge_release_CLIENTCONSUMER_RET_x64FRE_fr-fr.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "en-us"},
		{FileName: "26200.2.ge_release_CLIENTCONSUMER_RET_A64FRE_en-us.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "en-us"},
	}}

	got := cat.FilterARM64BuildMajor(26100, "Professional", "en-us")
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 ARM64 match, got %d: %+v", len(got), got)
	}
	if !got[0].IsARM64() || got[0].BuildMajor() != 26100 {
		t.Fatalf("unexpected match: %+v", got[0])
	}
}
