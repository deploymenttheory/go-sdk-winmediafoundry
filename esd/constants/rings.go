package constants

// Ring is the Windows Update release channel.
type Ring string

const (
	// RingCanary is the fastest-moving Insider channel (CanaryChannel SOAP value).
	RingCanary Ring = "Canary"

	// RingExperimental is the public name for the Dev Insider channel as of April 2026
	// (maps to "Dev" FlightingBranchName in the SOAP wire format).
	RingExperimental Ring = "Experimental"

	// RingDev is the deprecated alias for RingExperimental — kept for compatibility.
	RingDev Ring = RingExperimental

	// RingBeta is the Beta Insider channel (WIS SOAP value).
	RingBeta Ring = "Beta"

	// RingReleasePreview is the Release Preview Insider channel (RP SOAP value).
	RingReleasePreview Ring = "ReleasePreview"

	// RingRetail is the stable / generally available channel.
	RingRetail Ring = "Retail"

	// RingMSIT is the Microsoft internal channel.
	RingMSIT Ring = "MSIT"
)
