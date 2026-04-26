// Package buildver exposes the version string set at link time via -ldflags.
package buildver

// Version is injected by the build system via:
//
//	go build -ldflags "-X github.com/deploymenttheory/go-sdk-windowsuup/internal/buildver.Version=1.2.3"
var Version = "dev"
