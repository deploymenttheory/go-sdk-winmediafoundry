package constants

import "strings"

// Arch is a CPU architecture as surfaced by Microsoft's consumer download flow.
// Microsoft splits its Windows 11 ISO download pages by architecture (a separate
// page, and thus a separate product-edition id, for x64 and Arm64).
type Arch string

const (
	// ArchX64 is the Intel/AMD 64-bit architecture.
	ArchX64 Arch = "x64"
	// ArchARM64 is the 64-bit ARM architecture.
	ArchARM64 Arch = "ARM64"
)

// String returns the architecture as a plain string.
func (a Arch) String() string { return string(a) }

// ArchFromString normalises a free-form architecture token (e.g. "arm64",
// "Arm64", "aarch64", "amd64", "x86_64") to a known Arch, or "" if unrecognised.
func ArchFromString(s string) Arch {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "arm64", "aarch64", "a64":
		return ArchARM64
	case "x64", "amd64", "x86_64", "x86-64":
		return ArchX64
	default:
		return ""
	}
}

// ArchFromToken infers the architecture from any string that may embed an
// architecture token — a product-edition name, an ISO filename, or a download
// URL (e.g. "...Win11_25H2_English_Arm64_v2.iso"). It returns "" when no token
// is present.
func ArchFromToken(s string) Arch {
	up := strings.ToUpper(s)
	switch {
	case strings.Contains(up, "ARM64"), strings.Contains(up, "AARCH64"), strings.Contains(up, "A64"):
		return ArchARM64
	case strings.Contains(up, "X64"), strings.Contains(up, "AMD64"), strings.Contains(up, "X86_64"):
		return ArchX64
	default:
		return ""
	}
}
