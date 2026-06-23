package soap

import (
	"os"
	"testing"

	"github.com/deploymenttheory/winmediafoundry/internal/wuproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── parseSyncUpdatesResponse ────────────────────────────────────────────────

func TestParseSyncUpdatesResponse_FromFile(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/soap/fetch_updates_response.xml")
	require.NoError(t, err)

	results, err := parseSyncUpdatesResponse(raw)
	require.NoError(t, err)

	// Only the IsLeaf=true update should be returned.
	require.Len(t, results, 1, "expected exactly 1 leaf update (IsLeaf=false one excluded)")

	r := results[0]
	assert.Equal(t, "test-uuid-1234-5678-abcd-ef0123456789", r.UpdateID)
	assert.Equal(t, 200, r.Revision)
	assert.Equal(t, "Windows 11 Insider Preview Feature Update (26120.4061)", r.Title)
	assert.Equal(t, "10.0.26120.4061", r.Build)
	assert.Equal(t, wuproto.ArchAMD64, r.Arch)

	// EXPRESS file should be excluded, ESD should remain.
	require.Len(t, r.Files, 1, "EXPRESS cab should be filtered; ESD should remain")
	f := r.Files[0]
	assert.Equal(t, "Windows11.0-26120.4061-amd64.esd", f.Name)
	assert.EqualValues(t, 4294967296, f.SizeBytes)
	assert.NotEmpty(t, f.SHA1, "SHA1 should be decoded from base64 digest")
	assert.NotEmpty(t, f.SHA256, "SHA256 should be decoded from AdditionalDigest")
}

func TestParseSyncUpdatesResponse_Empty(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <SyncUpdatesResponse xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
      <SyncUpdatesResult>
        <NewUpdates/>
        <ExtendedUpdateInfo><Updates/></ExtendedUpdateInfo>
      </SyncUpdatesResult>
    </SyncUpdatesResponse>
  </s:Body>
</s:Envelope>`)

	results, err := parseSyncUpdatesResponse(raw)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestParseSyncUpdatesResponse_MalformedXML(t *testing.T) {
	_, err := parseSyncUpdatesResponse([]byte("not xml"))
	assert.Error(t, err)
}

// ─── branchFromBuild ─────────────────────────────────────────────────────────

func TestBranchFromBuild(t *testing.T) {
	tests := []struct {
		build  string
		branch string
	}{
		{"10.0.27000.1", "ge_prerelease"},
		{"10.0.26100.1", "ge_release"},
		{"10.0.25398.0", "zn_release"},
		{"10.0.22631.0", "ni_release"},
		{"10.0.22621.0", "ni_release"},
		{"10.0.22000.0", "co_release"},
		{"10.0.20348.0", "fe_release"},
		{"10.0.19041.0", "vb_release"},
		{"10.0.18362.0", "19h1_release"},
		{"10.0.17763.0", "rs5_release"},
		{"10.0.9600.0", "rs_prerelease"},
		{"bad", "rs_prerelease"},
		{"", "rs_prerelease"},
	}
	for _, tt := range tests {
		t.Run(tt.build, func(t *testing.T) {
			assert.Equal(t, tt.branch, branchFromBuild(tt.build))
		})
	}
}

// ─── isCookieError ───────────────────────────────────────────────────────────

func TestIsCookieError(t *testing.T) {
	assert.True(t, isCookieError("ConfigChanged"))
	assert.True(t, isCookieError("CookieExpired"))
	assert.True(t, isCookieError("InvalidCookie"))
	assert.True(t, isCookieError("<faultstring>configchanged</faultstring>"))
	assert.False(t, isCookieError("some other error"))
	assert.False(t, isCookieError(""))
}

// ─── base64ToHex ─────────────────────────────────────────────────────────────

func TestBase64ToHex(t *testing.T) {
	// base64("hello") = "aGVsbG8=" → hex of "hello" = "68656c6c6f"
	assert.Equal(t, "68656c6c6f", base64ToHex("aGVsbG8="))
	assert.Equal(t, "", base64ToHex(""))
	assert.Equal(t, "", base64ToHex("!!!not-base64"))
}

// ─── productNameToArch ───────────────────────────────────────────────────────

func TestProductNameToArch(t *testing.T) {
	assert.Equal(t, wuproto.ArchAMD64, productNameToArch("Client.OS.amd64"))
	assert.Equal(t, wuproto.ArchX86, productNameToArch("Client.OS.x86"))
	assert.Equal(t, wuproto.ArchARM64, productNameToArch("Client.OS.arm64"))
	assert.Equal(t, wuproto.ArchAMD64, productNameToArch("unknown")) // default
}

// ─── buildDeviceAttributes ───────────────────────────────────────────────────

func TestBuildDeviceAttributes_ContainsRequiredFields(t *testing.T) {
	attrs := buildDeviceAttributes("amd64", "Retail", "10.0.26100.1", "", 48, "Production")

	assert.Contains(t, attrs, "E:")
	assert.Contains(t, attrs, "OSArchitecture=amd64")
	assert.Contains(t, attrs, "OSSkuId=48")
	assert.Contains(t, attrs, "FlightRing=Retail")
	assert.Contains(t, attrs, "IsFlightingEnabled=0")
	assert.Contains(t, attrs, "IsRetailOS=1")
}

func TestBuildDeviceAttributes_ExperimentalRing(t *testing.T) {
	attrs := buildDeviceAttributes("amd64", "Experimental", "10.0.9600.0", "", 48, "Production")

	assert.Contains(t, attrs, "FlightingBranchName=Dev")
	assert.Contains(t, attrs, "FlightRing=External")
	assert.Contains(t, attrs, "IsFlightingEnabled=1")
}

func TestBuildDeviceAttributes_ServerSKU(t *testing.T) {
	attrs := buildDeviceAttributes("amd64", "Retail", "10.0.26100.1", "", 7, "Production")

	assert.Contains(t, attrs, "DeviceFamily=Windows.Server")
	assert.Contains(t, attrs, "InstallationType=Server")
	assert.Contains(t, attrs, "BlockFeatureUpdates=1")
}
