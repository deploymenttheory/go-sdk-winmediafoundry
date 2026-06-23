package builds

import (
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBuildVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    BuildVersion
		wantErr bool
	}{
		{
			input: "10.0.26100.4061",
			want:  BuildVersion{Major: 10, Minor: 0, Patch: 26100, Revision: 4061},
		},
		{
			input: "10.0.19041.1",
			want:  BuildVersion{Major: 10, Minor: 0, Patch: 19041, Revision: 1},
		},
		{
			input: "10.0.22621.3880",
			want:  BuildVersion{Major: 10, Minor: 0, Patch: 22621, Revision: 3880},
		},
		{
			input:   "10.0.26100",
			wantErr: true, // only 3 parts
		},
		{
			input:   "10.0.26100.4061.extra",
			wantErr: true, // 5 parts
		},
		{
			input:   "garbage",
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
		{
			input:   "10.0.abc.1",
			wantErr: true, // non-numeric patch
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBuildVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareBuilds(t *testing.T) {
	build := func(s string) models.Build { return models.Build{Build: s} }

	tests := []struct {
		a, b string
		want int
	}{
		{"10.0.26100.4061", "10.0.26100.4060", 1},  // a newer
		{"10.0.26100.4060", "10.0.26100.4061", -1}, // a older
		{"10.0.26100.4061", "10.0.26100.4061", 0},  // equal
		{"10.0.26200.1", "10.0.26100.9999", 1},     // newer patch wins
		{"10.0.19041.1", "10.0.22621.1", -1},       // older patch loses
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareBuilds(build(tt.a), build(tt.b))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareBuilds_ParseError(t *testing.T) {
	// On parse error, falls back to lexicographic comparison — must not panic.
	a := models.Build{Build: "garbage"}
	b := models.Build{Build: "also-garbage"}
	_ = CompareBuilds(a, b) // just confirm no panic
}

func TestCompareBuilds_SymmetricZero(t *testing.T) {
	build := models.Build{Build: "10.0.26100.4061"}
	assert.Equal(t, 0, CompareBuilds(build, build))
}
