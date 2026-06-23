package soap

import "encoding/xml"

// ─── GetCookie response ────────────────────────────────────────────────────

type getCookieEnvelope struct {
	XMLName xml.Name      `xml:"Envelope"`
	Body    getCookieBody `xml:"Body"`
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

// extUpdateInfo holds one <Update> entry from the ExtendedUpdateInfo section
// of the SyncUpdates response. Each numeric ID may appear twice — once with
// a LocalizedProperties blob and once with an ExtendedProperties+Files blob.
type extUpdateInfo struct {
	ID  int    `xml:"ID"`
	Xml string `xml:"Xml"` // HTML-escaped XML blob; parse via parseExtBlob
}

// extUpdateBlobFragment is parsed from the HTML-escaped <Xml> blob inside an
// ExtendedUpdateInfo <Update> element. The blob has no single root element so
// callers must wrap it in a synthetic <root>…</root> before unmarshalling.
type extUpdateBlobFragment struct {
	LocalizedProperties []localizedProp `xml:"LocalizedProperties"`
	Files               []extFile       `xml:"ExtendedProperties>Files>File"`
}

// extFile represents one <File> entry inside an ExtendedProperties blob.
type extFile struct {
	FileName          string          `xml:"FileName,attr"`
	Digest            string          `xml:"Digest,attr"` // base64-encoded SHA1
	DigestAlgorithm   string          `xml:"DigestAlgorithm,attr"`
	Size              int64           `xml:"Size,attr"`
	Modified          string          `xml:"Modified,attr"` // RFC3339
	AdditionalDigests []extFileDigest `xml:"AdditionalDigest"`
}

// extFileDigest holds one <AdditionalDigest Algorithm="SHA256">…</AdditionalDigest>.
type extFileDigest struct {
	Algorithm string `xml:"Algorithm,attr"`
	Value     string `xml:",chardata"`
}

// ─── Extended update blob (parsed from XmlUpdateBlob) ─────────────────────

// updateBlobFragment captures the fields relevant to update identity and
// platform applicability from the <Xml> blob inside each UpdateInfo.
type updateBlobFragment struct {
	UpdateIdentity updateIdentityXML `xml:"UpdateIdentity"`
	ProductRelease productRelease    `xml:"ApplicabilityRules>IsInstalled>ProductReleaseInstalled"`
}

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
	FileDigest string `xml:"FileDigest"` // base64-encoded SHA1
	URL        string `xml:"Url"`
}
