package soap

import "encoding/xml"

// ─── GetCookie response ────────────────────────────────────────────────────

type getCookieEnvelope struct {
	XMLName xml.Name        `xml:"Envelope"`
	Body    getCookieBody   `xml:"Body"`
}

type getCookieBody struct {
	Result getCookieResult `xml:"GetCookieResponse>GetCookieResult"`
}

type getCookieResult struct {
	EncryptedData string `xml:"EncryptedData"`
	Expiration    string `xml:"Expiration"`
}

// ─── SyncUpdates response ──────────────────────────────────────────────────

type syncUpdatesEnvelope struct {
	XMLName xml.Name        `xml:"Envelope"`
	Body    syncUpdatesBody `xml:"Body"`
}

type syncUpdatesBody struct {
	Result syncUpdatesResult `xml:"SyncUpdatesResponse>SyncUpdatesResult"`
}

type syncUpdatesResult struct {
	NewUpdates []syncUpdate    `xml:"NewUpdates>UpdateInfo"`
	ExtUpdates []extUpdateInfo `xml:"ExtendedUpdateInfo>Updates>Update"`
}

type syncUpdate struct {
	ID             int    `xml:"ID"`
	IsLeaf         string `xml:"IsLeaf"` // "true" | "false" | "" — use isLeafUpdate()
	RevisionNumber int    `xml:"RevisionNumber"`
	// XmlUpdateBlob contains the escaped XML fragment with update identity,
	// applicability rules, and properties. The element name in the WU SOAP
	// response is literally "<Xml>" (not "XmlUpdateBlob").
	XmlUpdateBlob string `xml:"Xml"`
}

// isLeafUpdate returns true when the WU service marked this update as a leaf
// (i.e. an installable package, not a category/detectoid).
func (u *syncUpdate) isLeafUpdate() bool {
	return u.IsLeaf == "true"
}

// extUpdateInfo holds the LocalizedProperties for one update from the
// ExtendedUpdateInfo section of the SyncUpdates response.
type extUpdateInfo struct {
	ID                  int             `xml:"ID"`
	LocalizedProperties []localizedProp `xml:"LocalizedProperties"`
}

// ─── Extended update blob (parsed from XmlUpdateBlob) ─────────────────────
//
// The blob has no single root element; callers must wrap it in a synthetic
// <root>…</root> before unmarshalling.

// updateBlobFragment captures the fields relevant to update identity and
// platform applicability from the <Xml> blob inside each UpdateInfo.
type updateBlobFragment struct {
	// UpdateIdentity carries the real UUID and revision number.
	UpdateIdentity updateIdentityXML `xml:"UpdateIdentity"`
	// ProductRelease (via ApplicabilityRules > IsInstalled) holds the
	// product name and OS version that determine architecture and build.
	ProductRelease productRelease `xml:"ApplicabilityRules>IsInstalled>ProductReleaseInstalled"`
}

// updateIdentityXML represents the <UpdateIdentity> element inside the blob.
type updateIdentityXML struct {
	UpdateID       string `xml:"UpdateID,attr"`
	RevisionNumber int    `xml:"RevisionNumber,attr"`
}

type localizedProp struct {
	Language string `xml:"Language"`
	Title    string `xml:"Title"`
}

type productRelease struct {
	Name    string `xml:"Name,attr"`
	Version string `xml:"Version,attr"`
}

// ─── GetExtendedUpdateInfo2 response ──────────────────────────────────────

type getEUI2Envelope struct {
	XMLName xml.Name    `xml:"Envelope"`
	Body    getEUI2Body `xml:"Body"`
}

type getEUI2Body struct {
	Result getEUI2Result `xml:"GetExtendedUpdateInfo2Response>GetExtendedUpdateInfo2Result"`
}

type getEUI2Result struct {
	FileLocations []fileLocation `xml:"FileLocations>FileLocation"`
}

type fileLocation struct {
	// FileDigest is a base64-encoded SHA1 hash.
	FileDigest string `xml:"FileDigest"`
	URL        string `xml:"Url"`
}
