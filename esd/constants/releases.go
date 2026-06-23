package constants

import "strings"

// Release is a Windows 11 feature-update marketing name (e.g. "24H2"). The MCT
// ESD catalog does not carry this name directly; each ESD filename instead
// begins with the base build number (e.g. "26100.4349..."), so a release name
// is resolved to its build number via ReleaseBuild and then matched against the
// catalog (see models.ESDCatalog.FilterBuildMajor).
type Release string

// Known Windows 11 feature releases.
const (
	Release21H2 Release = "21H2"
	Release22H2 Release = "22H2"
	Release23H2 Release = "23H2"
	Release24H2 Release = "24H2"
	Release25H2 Release = "25H2"
)

// windows11ReleaseBuilds maps a Windows 11 feature release to its base build
// number — the leading number of every ESD filename for that release. Update
// this table as Microsoft ships new feature updates.
var windows11ReleaseBuilds = map[Release]int{
	Release21H2: 22000,
	Release22H2: 22621,
	Release23H2: 22631,
	Release24H2: 26100,
	Release25H2: 26200,
}

// ReleaseBuild returns the base build number for a Windows 11 feature release
// and whether the release name is known. Matching is case-insensitive, so
// "25h2" and "25H2" both resolve.
//
// Note: a known release is not guaranteed to be present in the MCT catalog —
// the catalog typically carries only the current GA release. Resolve the build
// here, then confirm availability with models.ESDCatalog.FilterBuildMajor.
func ReleaseBuild(r Release) (int, bool) {
	b, ok := windows11ReleaseBuilds[Release(strings.ToUpper(string(r)))]
	return b, ok
}

// Releases returns the known Windows 11 feature release names, unordered.
func Releases() []Release {
	out := make([]Release, 0, len(windows11ReleaseBuilds))
	for r := range windows11ReleaseBuilds {
		out = append(out, r)
	}
	return out
}
