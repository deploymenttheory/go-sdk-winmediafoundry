package builds

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
)

// BuildVersion holds the four numeric components of a Windows build string
// such as "10.0.26100.4061".
type BuildVersion struct {
	Major    int // always 10 for modern Windows
	Minor    int // always 0 for modern Windows
	Patch    int // the build number, e.g. 26100
	Revision int // the update revision, e.g. 4061
}

// ParseBuildVersion parses a dot-separated Windows build string such as
// "10.0.26100.4061" into its four numeric components.
// Returns an error if the string does not contain exactly four dot-separated
// integers.
func ParseBuildVersion(s string) (BuildVersion, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return BuildVersion{}, fmt.Errorf("ParseBuildVersion: expected 4 dot-separated parts, got %d in %q", len(parts), s)
	}

	var v BuildVersion
	var err error

	if v.Major, err = strconv.Atoi(parts[0]); err != nil {
		return BuildVersion{}, fmt.Errorf("ParseBuildVersion: invalid major %q in %q", parts[0], s)
	}
	if v.Minor, err = strconv.Atoi(parts[1]); err != nil {
		return BuildVersion{}, fmt.Errorf("ParseBuildVersion: invalid minor %q in %q", parts[1], s)
	}
	if v.Patch, err = strconv.Atoi(parts[2]); err != nil {
		return BuildVersion{}, fmt.Errorf("ParseBuildVersion: invalid patch %q in %q", parts[2], s)
	}
	if v.Revision, err = strconv.Atoi(parts[3]); err != nil {
		return BuildVersion{}, fmt.Errorf("ParseBuildVersion: invalid revision %q in %q", parts[3], s)
	}

	return v, nil
}

// CompareBuilds compares two builds by version number.
// Returns -1 if a is older than b, 0 if equal, +1 if a is newer than b.
//
// This function is compatible with slices.SortFunc:
//
//	slices.SortFunc(builds, buildsapi.CompareBuilds)
//
// On parse error (malformed build string) it falls back to a lexicographic
// comparison of the raw Build strings.
func CompareBuilds(a, b models.Build) int {
	va, errA := ParseBuildVersion(a.Build)
	vb, errB := ParseBuildVersion(b.Build)

	if errA != nil || errB != nil {
		return strings.Compare(a.Build, b.Build)
	}

	for _, pair := range [4][2]int{
		{va.Major, vb.Major},
		{va.Minor, vb.Minor},
		{va.Patch, vb.Patch},
		{va.Revision, vb.Revision},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
