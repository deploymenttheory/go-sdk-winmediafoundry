package constants

// Arch is the CPU architecture passed to the Windows Update SOAP service.
type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchX86   Arch = "x86"
	ArchARM64 Arch = "arm64"
)
