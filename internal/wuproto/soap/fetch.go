package soap

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
)

// parseExtBlob parses the HTML-escaped <Xml> blob from an ExtendedUpdateInfo
// Update entry. Returns nil (not an error) when the blob is empty or unparseable.
func parseExtBlob(blob string) *extUpdateBlobFragment {
	if blob == "" {
		return nil
	}
	wrapped := "<root>" + blob + "</root>"
	var frag extUpdateBlobFragment
	if err := xml.Unmarshal([]byte(wrapped), &frag); err != nil {
		return nil
	}
	return &frag
}

// ringToFlightBranch maps public ring names (and legacy aliases) to the
// FlightingBranchName value sent in SOAP device attributes.
//
// April 2026: Microsoft renamed the Dev channel to "Experimental" publicly,
// but the underlying SOAP FlightingBranchName value remains "Dev".
var ringToFlightBranch = map[string]string{
	"Canary":         "CanaryChannel",
	"Experimental":   "Dev", // public name as of April 2026
	"Dev":            "Dev", // legacy alias
	"WIF":            "Dev", // legacy SOAP alias
	"Beta":           "WIS",
	"WIS":            "WIS", // legacy SOAP alias
	"ReleasePreview": "RP",
	"RP":             "RP", // legacy SOAP alias
	"Retail":         "",
	"MSIT":           "MSIT",
}

// skuToProduct maps Windows SKU IDs to the WU product name prefix.
var skuToProduct = map[int]string{
	// Server SKUs
	7: "Server.OS", 8: "Server.OS", 12: "Server.OS", 13: "Server.OS",
	79: "Server.OS", 80: "Server.OS", 120: "Server.OS",
	145: "Server.OS", 146: "Server.OS", 147: "Server.OS", 148: "Server.OS",
	159: "Server.OS", 160: "Server.OS", 406: "Server.OS", 407: "Server.OS", 408: "Server.OS",
	// Special devices
	180: "WCOSDevice2.OS",
	184: "WCOSDevice1.OS",
	189: "WCOSDevice0.OS",
	210: "WNC.OS",
}

// DefaultCheckBuild returns the appropriate default OS-version string to use in
// SOAP device/product attributes when the caller has not specified one.
//
// Flight/Insider rings (Experimental, Beta, ReleasePreview, Canary, MSIT) carry
// an explicit FlightingBranchName in the SOAP request, so an old legacy build
// string is sufficient to trigger WU's upgrade path.
//
// The Retail ring derives its products Branch from branchFromBuild, so a recent
// Windows 11 build string is required to produce Branch=ge_release (or similar)
// and receive current stable builds.
func DefaultCheckBuild(ring string) string {
	if ringToFlightBranch[ring] != "" {
		return "10.0.9600.0" // Insider/flight rings: explicit FlightingBranchName overrides
	}
	return "10.0.26100.1" // Retail: Win11 24H2 GA → Branch=ge_release
}

// IsCookieError returns true when the WU error response body indicates a
// cookie / config problem that warrants a cookie refresh and retry.
var cookieErrorRegexp = regexp.MustCompile(`(?i)(ConfigChanged|CookieExpired|InvalidCookie)`)

func IsCookieError(body string) bool {
	return cookieErrorRegexp.MatchString(body)
}

// ParseSyncUpdatesResponse extracts UpdateResult values from a raw SyncUpdates XML response.
func ParseSyncUpdatesResponse(raw []byte) ([]wuproto.UpdateResult, error) {
	var env syncUpdatesEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal SyncUpdates response: %w", err)
	}

	type extInfo struct {
		title string
		files []wuproto.FileMetadata
	}
	extByID := make(map[int]*extInfo, len(env.Body.Result.ExtUpdates))
	for _, ext := range env.Body.Result.ExtUpdates {
		frag := parseExtBlob(ext.Xml)
		if frag == nil {
			continue
		}
		ei := extByID[ext.ID]
		if ei == nil {
			ei = &extInfo{}
			extByID[ext.ID] = ei
		}
		if ei.title == "" {
			for _, lp := range frag.LocalizedProperties {
				if lp.Title != "" {
					ei.title = lp.Title
					break
				}
			}
		}
		for _, f := range frag.Files {
			if shouldExclude(f.FileName) {
				continue
			}
			fm := wuproto.FileMetadata{
				Name:      f.FileName,
				SHA1:      base64ToHex(f.Digest),
				SizeBytes: f.Size,
			}
			if f.Modified != "" {
				if t, err := time.Parse(time.RFC3339Nano, f.Modified); err == nil {
					fm.Modified = t
				} else if t, err := time.Parse(time.RFC3339, f.Modified); err == nil {
					fm.Modified = t
				}
			}
			for _, ad := range f.AdditionalDigests {
				if ad.Algorithm == "SHA256" {
					fm.SHA256 = base64ToHex(ad.Value)
					break
				}
			}
			ei.files = append(ei.files, fm)
		}
	}

	now := time.Now().UTC()
	var results []wuproto.UpdateResult

	for _, upd := range env.Body.Result.NewUpdates {
		if !upd.isLeafUpdate() {
			continue
		}
		if upd.XmlUpdateBlob == "" {
			continue
		}

		blob, err := parseUpdateBlob(upd.XmlUpdateBlob)
		if err != nil {
			continue
		}

		updateID := blob.UpdateIdentity.UpdateID
		revision := blob.UpdateIdentity.RevisionNumber
		if updateID == "" {
			updateID = fmt.Sprintf("%d", upd.ID)
		}
		if revision == 0 {
			revision = upd.RevisionNumber
		}

		ei := extByID[upd.ID]

		result := wuproto.UpdateResult{
			UpdateID:     updateID,
			Revision:     revision,
			DiscoveredAt: now,
		}
		if ei != nil {
			result.Title = ei.title
			result.Files = ei.files
		}

		result.Build = blob.ProductRelease.Version
		result.Arch = productNameToArch(blob.ProductRelease.Name)

		results = append(results, result)
	}

	return results, nil
}

// parseUpdateBlob parses the <Xml> blob field from an UpdateInfo element.
func parseUpdateBlob(blob string) (*updateBlobFragment, error) {
	wrapped := "<root>" + blob + "</root>"
	var frag updateBlobFragment
	if err := xml.Unmarshal([]byte(wrapped), &frag); err != nil {
		return nil, err
	}
	return &frag, nil
}

// base64ToHex converts a base64-encoded string to a lowercase hex string.
func base64ToHex(b64 string) string {
	if b64 == "" {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(data)
}

// productNameToArch maps a WU product name (e.g. "Client.OS.amd64") to an Arch.
func productNameToArch(pn string) wuproto.Arch {
	switch {
	case strings.HasSuffix(pn, ".amd64"):
		return wuproto.ArchAMD64
	case strings.HasSuffix(pn, ".x86"):
		return wuproto.ArchX86
	case strings.HasSuffix(pn, ".arm64"):
		return wuproto.ArchARM64
	default:
		return wuproto.ArchAMD64
	}
}

// BuildDeviceAttributes constructs the E: device attributes string sent in
// SyncUpdates and GetExtendedUpdateInfo2. Ported from UUP dump PHP reference
// (shared/requests.php composeDeviceAttributes).
func BuildDeviceAttributes(arch, ring, build, _ string, sku int, buildType string) string {
	blockUpgrades := "0"
	flightEnabled := "1"
	isRetail := "0"
	dvcFamily := "Windows.Desktop"
	insType := "Client"
	prodType := "WinNT"

	if sku == 125 || sku == 126 {
		blockUpgrades = "1"
	}
	if isServerSKU(sku) {
		dvcFamily = "Windows.Server"
		insType = "Server"
		prodType = "ServerNT"
		blockUpgrades = "1"
	}

	fltBranch := ""
	fltRing := "External"

	switch ring {
	case "RETAIL", "Retail":
		flightEnabled = "0"
		isRetail = "1"
		fltRing = "Retail"
	case "WIF", "Dev", "Experimental":
		fltBranch = "Dev"
	case "WIS", "Beta":
		fltBranch = "WIS"
	case "RP", "ReleasePreview":
		fltBranch = "RP"
	case "Canary", "CANARY":
		fltBranch = "CanaryChannel"
	case "MSIT":
		fltBranch = "MSIT"
		fltRing = "Internal"
	}

	branch := branchFromBuild(build)

	now := time.Now().Unix()
	dataExp := fmt.Sprintf("%d", now+82800)
	timeStamp := fmt.Sprintf("%d", now-3600)

	if buildType == "" {
		buildType = "Production"
	}

	parts := []string{
		"App=WU_OS",
		"AppVer=" + build,
		"AttrDataVer=352",
		"AllowInPlaceUpgrade=1",
		"AllowOptionalContent=1",
		"AllowUpgradesWithUnsupportedTPMOrCPU=1",
		"BlockFeatureUpdates=" + blockUpgrades,
		"BranchReadinessLevel=CB",
		"CIOptin=1",
		"CurrentBranch=" + branch,
		"DataExpDateEpoch_GE25H2=" + dataExp,
		"DataExpDateEpoch_GE24H2=" + dataExp,
		"DataExpDateEpoch_GE24H2Setup=" + dataExp,
		"DataExpDateEpoch_CU23H2=" + dataExp,
		"DataExpDateEpoch_CU23H2Setup=" + dataExp,
		"DataExpDateEpoch_NI22H2=" + dataExp,
		"DataExpDateEpoch_NI22H2Setup=" + dataExp,
		"DataExpDateEpoch_CO21H2=" + dataExp,
		"DataExpDateEpoch_CO21H2Setup=" + dataExp,
		"DataExpDateEpoch_23H2=" + dataExp,
		"DataExpDateEpoch_22H2=" + dataExp,
		"DataExpDateEpoch_21H2=" + dataExp,
		"DataExpDateEpoch_21H1=" + dataExp,
		"DataExpDateEpoch_20H1=" + dataExp,
		"DataExpDateEpoch_19H1=" + dataExp,
		"DataVer_RS5=2000000000",
		"DefaultUserRegion=191",
		"DeviceFamily=" + dvcFamily,
		"DeviceInfoGatherSuccessful=1",
		"EKB19H2InstallCount=1",
		"EKB19H2InstallTimeEpoch=1255000000",
		"FlightingBranchName=" + fltBranch,
		"FlightRing=" + fltRing,
		"Free=gt64",
		"GStatus_GE25H2=2",
		"GStatus_GE24H2=2",
		"GStatus_GE24H2Setup=2",
		"GStatus_CU23H2=2",
		"GStatus_CU23H2Setup=2",
		"GStatus_NI23H2=2",
		"GStatus_NI22H2=2",
		"GStatus_NI22H2Setup=2",
		"GStatus_CO21H2=2",
		"GStatus_CO21H2Setup=2",
		"GStatus_22H2=2",
		"GStatus_21H2=2",
		"GStatus_21H1=2",
		"GStatus_20H1=2",
		"GStatus_20H1Setup=2",
		"GStatus_19H1=2",
		"GStatus_19H1Setup=2",
		"GStatus_RS5=2",
		"GenTelRunTimestamp_19H1=" + timeStamp,
		"InstallDate=1438196400",
		"InstallLanguage=en-US",
		"InstallationType=" + insType,
		"IsDeviceRetailDemo=0",
		"IsFlightingEnabled=" + flightEnabled,
		"IsRetailOS=" + isRetail,
		"LaunchUserOOBE=1",
		"LCUVer=0.0.0.0",
		"MediaBranch=",
		"MediaVersion=" + build,
		"CloudPBR=1",
		"DUScan=1",
		"OEMModel=21F6CTO1WW",
		"OEMModelBaseBoard=21F6CTO1WW",
		"OEMName_Uncleaned=LENOVO",
		"OemPartnerRing=UPSFlighting",
		"OSArchitecture=" + arch,
		fmt.Sprintf("OSSkuId=%d", sku),
		"OSUILocale=en-US",
		"OSVersion=" + build,
		"ProcessorIdentifier=Intel64 Family 6 Model 186 Stepping 3",
		"ProcessorManufacturer=GenuineIntel",
		"ProcessorModel=13th Gen Intel(R) Core(TM) i7-1355U",
		"ProductType=" + prodType,
		"ReleaseType=" + buildType,
		"SdbVer_20H1=2000000000",
		"SdbVer_19H1=2000000000",
		"SecureBootCapable=1",
		"TelemetryLevel=3",
		"TimestampEpochString_GE24H2=" + timeStamp,
		"TimestampEpochString_GE24H2Setup=" + timeStamp,
		"TimestampEpochString_CU23H2=" + timeStamp,
		"TimestampEpochString_CU23H2Setup=" + timeStamp,
		"TimestampEpochString_NI23H2=" + timeStamp,
		"TimestampEpochString_NI22H2=" + timeStamp,
		"TimestampEpochString_NI22H2Setup=" + timeStamp,
		"TimestampEpochString_CO21H2=" + timeStamp,
		"TimestampEpochString_CO21H2Setup=" + timeStamp,
		"TimestampEpochString_22H2=" + timeStamp,
		"TimestampEpochString_21H2=" + timeStamp,
		"TimestampEpochString_21H1=" + timeStamp,
		"TimestampEpochString_20H1=" + timeStamp,
		"TimestampEpochString_19H1=" + timeStamp,
		"TPMVersion=2",
		"UpdateManagementGroup=2",
		"UpdateOfferedDays=0",
		"UpgEx_GE25H2=Green",
		"UpgEx_GE24H2Setup=Green",
		"UpgEx_GE24H2=Green",
		"UpgEx_CU23H2=Green",
		"UpgEx_NI23H2=Green",
		"UpgEx_NI22H2=Green",
		"UpgEx_CO21H2=Green",
		"UpgEx_23H2=Green",
		"UpgEx_22H2=Green",
		"UpgEx_21H2=Green",
		"UpgEx_21H1=Green",
		"UpgEx_20H1=Green",
		"UpgEx_19H1=Green",
		"UpgEx_RS5=Green",
		"UpgradeAccepted=1",
		"UpgradeEligible=1",
		"UserInPlaceUpgrade=1",
		"VBSState=2",
		"Version_RS5=2000000000",
		"Win10CommercialAzureESUEligible=1",
		"Win10CommercialKeybasedESUEligible=1",
		"Win10CommercialW365ESUEligible=1",
		"Win10ConsumerESUStatus=3",
		"Win10ConsumerESUAY=9",
		"WuClientVer=" + build,
	}

	return "E:" + strings.Join(parts, "&")
}

// isServerSKU returns true for Windows Server SKU identifiers.
func isServerSKU(sku int) bool {
	serverSKUs := map[int]bool{
		7: true, 8: true, 12: true, 13: true, 79: true, 80: true, 120: true,
		145: true, 146: true, 147: true, 148: true, 159: true, 160: true,
		406: true, 407: true, 408: true,
	}
	return serverSKUs[sku]
}

// branchFromBuild derives the Windows branch name from a build string like
// "10.0.26100.0". Ported from UUP dump PHP reference (branchFromBuild).
func branchFromBuild(build string) string {
	parts := strings.Split(build, ".")
	if len(parts) < 3 {
		return "rs_prerelease"
	}
	var bldnum int
	fmt.Sscanf(parts[2], "%d", &bldnum)

	switch {
	case bldnum >= 27000:
		return "ge_prerelease"
	case bldnum >= 26100:
		return "ge_release"
	case bldnum >= 25398:
		return "zn_release"
	case bldnum >= 22631:
		return "ni_release"
	case bldnum >= 22621:
		return "ni_release"
	case bldnum >= 22000:
		return "co_release"
	case bldnum >= 20348:
		return "fe_release"
	case bldnum >= 19041:
		return "vb_release"
	case bldnum >= 18362:
		return "19h1_release"
	case bldnum >= 17763:
		return "rs5_release"
	default:
		return "rs_prerelease"
	}
}

// BuildProductsString constructs the full products string for SyncUpdates.
// Ported from UUP dump PHP source (shared/requests.php composeFetchUpdRequest).
func BuildProductsString(arch, ring, build string, sku int) string {
	productPrefix, ok := skuToProduct[sku]
	if !ok {
		productPrefix = "Client.OS"
	}
	pn := productPrefix + "." + arch

	branch := ringToFlightBranch[ring]
	if branch == "" {
		branch = branchFromBuild(build)
	}

	products := []string{
		fmt.Sprintf("PN=%s&Branch=%s&PrimaryOSProduct=1&Repairable=1&V=%s&ReofferUpdate=1", pn, branch, build),
		"PN=Adobe.Flash." + arch + "&Repairable=1&V=0.0.0.0",
		"PN=Microsoft.Edge.Stable." + arch + "&Repairable=1&V=0.0.0.0",
		"PN=Microsoft.NETFX." + arch + "&V=0.0.0.0",
		"PN=Windows.Autopilot." + arch + "&Repairable=1&V=0.0.0.0",
		"PN=Windows.AutopilotOOBE." + arch + "&Repairable=1&V=0.0.0.0",
		"PN=Windows.Appraiser." + arch + "&Repairable=1&V=" + build,
		"PN=Windows.AppraiserData." + arch + "&Repairable=1&V=" + build,
		"PN=Windows.EmergencyUpdate." + arch + "&V=" + build,
		"PN=Windows.FeatureExperiencePack." + arch + "&Repairable=1&V=0.0.0.0",
		"PN=Windows.ManagementOOBE." + arch + "&IsWindowsManagementOOBE=1&Repairable=1&V=" + build,
		"PN=Windows.OOBE." + arch + "&IsWindowsOOBE=1&Repairable=1&V=" + build,
		"PN=Windows.OOBE.Cumulative." + arch + "&V=0.0.0.0",
		"PN=Windows.OOBE.Standalone." + arch + "&V=0.0.0.0",
		"PN=Windows.UpdateStackPackage." + arch + "&Name=Update Stack Package&Repairable=1&V=" + build,
		"PN=Hammer." + arch + "&Source=UpdateOrchestrator&V=0.0.0.0",
		"PN=MSRT." + arch + "&Source=UpdateOrchestrator&V=0.0.0.0",
		"PN=SedimentPack." + arch + "&Source=UpdateOrchestrator&V=0.0.0.0",
		"PN=UUS." + arch + "&Source=UpdateOrchestrator&V=0.0.0.0",
	}

	return strings.Join(products, ";")
}

// isCookieError is the unexported alias kept for internal package tests.
// New code should use IsCookieError.
func isCookieError(body string) bool { return IsCookieError(body) }

// parseSyncUpdatesResponse is the unexported alias kept for internal package tests.
func parseSyncUpdatesResponse(raw []byte) ([]wuproto.UpdateResult, error) {
	return ParseSyncUpdatesResponse(raw)
}

// buildDeviceAttributes is the unexported alias kept for internal package tests.
func buildDeviceAttributes(arch, ring, build, extra string, sku int, buildType string) string {
	return BuildDeviceAttributes(arch, ring, build, extra, sku, buildType)
}
