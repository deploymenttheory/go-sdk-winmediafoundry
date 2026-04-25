package soap

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/wuproto"
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

// Ring → flight branch alias (used in DeviceAttributes and Products string).
var ringToFlightBranch = map[string]string{
	"Canary":         "Canary",
	"Dev":            "WIF",
	"Beta":           "WIS",
	"WIF":            "WIF",
	"WIS":            "WIS",
	"ReleasePreview": "RP",
	"RP":             "RP",
	"Retail":         "",
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

// fetchUpdates implements the SyncUpdates SOAP call.
func (c *SOAPClient) fetchUpdates(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, error) {
	cookie, err := c.cookies.get(ctx)
	if err != nil {
		return nil, err
	}

	arch := string(req.Arch)
	if arch == "" {
		arch = "amd64"
	}

	ring := string(req.Ring)

	// checkBuild is the OS version the device claims to be on.
	//
	// For Insider rings (Dev/Beta/RP) an old Win10 build ("10.0.16251.0") causes
	// WU to offer the current Insider builds because those rings use explicit
	// FlightBranch values (WIF/WIS/RP) that override branch matching.
	//
	// For Retail ring the products string branch is derived from checkBuild via
	// branchFromBuild(). If checkBuild is a legacy build (<17763 → rs_prerelease)
	// WU marks the result OutOfScopeRevisionIDs because current Retail updates
	// target ge_release (Win11 24H2). We therefore default to the Win11 24H2 GA
	// build (10.0.26100.1) for non-flight rings so that CurrentBranch=ge_release
	// and Branch=ge_release are both set correctly.
	checkBuild := req.CheckBuild
	if checkBuild == "" {
		flightBranch := ringToFlightBranch[ring]
		if flightBranch != "" {
			// Insider/flight ring: use old build to trigger feature-upgrade path.
			checkBuild = "10.0.16251.0"
		} else {
			// Retail / non-flight ring: target Win11 24H2 GA → cumulative updates.
			checkBuild = "10.0.26100.1"
		}
	}

	buildType := string(req.Type)
	if buildType == "" {
		buildType = "Production"
	}

	sku := req.SKU
	if sku == 0 {
		sku = 48 // Windows 11 Professional
	}

	deviceAttrs := buildDeviceAttributes(arch, ring, checkBuild, "", sku, buildType)
	callerAttrs := "E:Profile=AUv2&Acquisition=1&Interactive=1&IsSeeker=1&SheddingAware=1&Id=MoUpdateOrchestrator"
	products := buildProductsString(arch, ring, checkBuild, sku)
	syncCurrentOnly := req.Build != ""

	body := buildSyncUpdatesEnvelope(
		time.Now(),
		c.cookies.deviceToken,
		cookie,
		deviceAttrs,
		callerAttrs,
		products,
		syncCurrentOnly,
	)

	resp, err := c.cookies.post(ctx, clientEndpoint,
		"http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/SyncUpdates",
		body)
	if err != nil {
		return nil, fmt.Errorf("SyncUpdates SOAP call: %w", err)
	}
	defer resp.Body.Close()

	// On HTTP 500 with cookie/config errors, invalidate and retry once.
	if resp.StatusCode == http.StatusInternalServerError {
		raw, _ := io.ReadAll(resp.Body)
		if isCookieError(string(raw)) {
			c.logger.Warn("WU cookie error, refreshing and retrying")
			c.cookies.invalidate()
			cookie, err = c.cookies.get(ctx)
			if err != nil {
				return nil, fmt.Errorf("cookie refresh after error: %w", err)
			}
			body = buildSyncUpdatesEnvelope(time.Now(), c.cookies.deviceToken, cookie,
				deviceAttrs, callerAttrs, products, syncCurrentOnly)
			resp, err = c.cookies.post(ctx, clientEndpoint,
				"http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/SyncUpdates",
				body)
			if err != nil {
				return nil, fmt.Errorf("SyncUpdates retry: %w", err)
			}
			defer resp.Body.Close()
		}
	}

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SyncUpdates returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SyncUpdates response: %w", err)
	}

	return parseSyncUpdatesResponse(raw)
}

// isCookieError returns true when the WU error response indicates a cookie/
// config problem that warrants a cookie refresh.
var cookieErrorRegexp = regexp.MustCompile(`(?i)(ConfigChanged|CookieExpired|InvalidCookie)`)

func isCookieError(body string) bool {
	return cookieErrorRegexp.MatchString(body)
}

// parseSyncUpdatesResponse extracts UpdateResult values from a raw SyncUpdates XML response.
func parseSyncUpdatesResponse(raw []byte) ([]wuproto.UpdateResult, error) {
	var env syncUpdatesEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal SyncUpdates response: %w", err)
	}

	// Parse ExtendedUpdateInfo blobs — each numeric ID may appear twice:
	// once with LocalizedProperties (title) and once with ExtendedProperties+Files.
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
		// Title from LocalizedProperties.
		if ei.title == "" {
			for _, lp := range frag.LocalizedProperties {
				if lp.Title != "" {
					ei.title = lp.Title
					break
				}
			}
		}
		// File metadata from ExtendedProperties > Files.
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

		// Real UUID and revision live inside the blob's <UpdateIdentity>.
		updateID := blob.UpdateIdentity.UpdateID
		revision := blob.UpdateIdentity.RevisionNumber
		if updateID == "" {
			updateID = fmt.Sprintf("%d", upd.ID) // fallback to numeric ID
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

		// Arch and build version from ApplicabilityRules > IsInstalled > ProductReleaseInstalled.
		result.Build = blob.ProductRelease.Version
		result.Arch = productNameToArch(blob.ProductRelease.Name)

		results = append(results, result)
	}

	return results, nil
}

// parseUpdateBlob parses the <Xml> blob field from an UpdateInfo element.
// The blob has no single root element, so it is wrapped in a synthetic
// <root> element before unmarshalling.
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

// productNameToArch maps a WU product name (e.g. "Client.OS.amd64") to a
// wuproto.Arch string.
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

// buildDeviceAttributes constructs the E: device attributes string sent in
// SyncUpdates and GetExtendedUpdateInfo2. The values are ported directly from
// the UUP dump PHP reference (shared/requests.php composeDeviceAttributes).
func buildDeviceAttributes(arch, ring, build, _ string, sku int, buildType string) string {
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

	// Normalise ring aliases and set flight branch / ring.
	switch ring {
	case "RETAIL", "Retail":
		flightEnabled = "0"
		isRetail = "1"
		fltRing = "Retail"
	case "WIF", "Dev":
		fltBranch = "Dev"
	case "WIS", "Beta":
		fltBranch = "Beta"
	case "RP", "ReleasePreview":
		fltBranch = "ReleasePreview"
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
// "10.0.26100.0". The mapping follows the UUP dump PHP reference (branchFromBuild).
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

// buildProductsString constructs the full products string for SyncUpdates.
// Ported from UUP dump PHP source (shared/requests.php composeFetchUpdRequest).
func buildProductsString(arch, ring, build string, sku int) string {
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
