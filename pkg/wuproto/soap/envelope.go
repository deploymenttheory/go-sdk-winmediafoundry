package soap

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// w3cTime formats a time.Time as a W3C / ISO 8601 timestamp with UTC offset.
func w3cTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05+00:00")
}

// newMessageID generates a random UUID v4 for use as a SOAP message ID.
func newMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// buildGetCookieEnvelope returns the SOAP XML body for a GetCookie request.
// Unexported — used only by CookieManager.acquire.
func buildGetCookieEnvelope(now time.Time, deviceToken string) string {
	msgID := newMessageID()
	created := w3cTime(now)
	expires := w3cTime(now.Add(120 * time.Second))

	return fmt.Sprintf(`<s:Envelope xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetCookie</a:Action>
    <a:MessageID>urn:uuid:%s</a:MessageID>
    <a:To s:mustUnderstand="1">%s</a:To>
    <o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <Timestamp xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
        <Created>%s</Created>
        <Expires>%s</Expires>
      </Timestamp>
      <wuws:WindowsUpdateTicketsToken wsu:id="ClientMSA" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd" xmlns:wuws="http://schemas.microsoft.com/msus/2014/10/WindowsUpdateAuthorization">
        <TicketType Name="MSA" Version="1.0" Policy="MBI_SSL">
          <Device>%s</Device>
        </TicketType>
      </wuws:WindowsUpdateTicketsToken>
    </o:Security>
  </s:Header>
  <s:Body>
    <GetCookie xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
      <oldCookie><Expiration>%s</Expiration></oldCookie>
      <lastChange>%s</lastChange>
      <currentTime>%s</currentTime>
      <protocolVersion>2.0</protocolVersion>
    </GetCookie>
  </s:Body>
</s:Envelope>`,
		msgID, ClientEndpoint,
		created, expires,
		deviceToken,
		created, created, created,
	)
}

// BuildSyncUpdatesEnvelope returns the SOAP XML body for a SyncUpdates request.
//
// encryptedData and cookieExpiration come from CookieManager.Get().
// deviceAttrs is built with BuildDeviceAttributes; products with BuildProductsString.
func BuildSyncUpdatesEnvelope(
	now time.Time,
	deviceToken string,
	encryptedData string,
	cookieExpiration string,
	deviceAttrs string,
	callerAttrs string,
	products string,
	syncCurrentOnly bool,
) string {
	msgID := newMessageID()
	created := w3cTime(now)
	expires := w3cTime(now.Add(120 * time.Second))

	syncCurrent := "false"
	if syncCurrentOnly {
		syncCurrent = "true"
	}

	installedIDs := buildInstalledUpdateIDsXML()

	return fmt.Sprintf(`<s:Envelope xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/SyncUpdates</a:Action>
    <a:MessageID>urn:uuid:%s</a:MessageID>
    <a:To s:mustUnderstand="1">%s</a:To>
    <o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <Timestamp xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
        <Created>%s</Created>
        <Expires>%s</Expires>
      </Timestamp>
      <wuws:WindowsUpdateTicketsToken wsu:id="ClientMSA" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd" xmlns:wuws="http://schemas.microsoft.com/msus/2014/10/WindowsUpdateAuthorization">
        <TicketType Name="MSA" Version="1.0" Policy="MBI_SSL">
          <Device>%s</Device>
        </TicketType>
      </wuws:WindowsUpdateTicketsToken>
    </o:Security>
  </s:Header>
  <s:Body>
    <SyncUpdates xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
      <cookie>
        <Expiration>%s</Expiration>
        <EncryptedData>%s</EncryptedData>
      </cookie>
      <parameters>
        <ExpressQuery>false</ExpressQuery>
        <InstalledNonLeafUpdateIDs>%s</InstalledNonLeafUpdateIDs>
        <OtherCachedUpdateIDs/>
        <SkipSoftwareSync>false</SkipSoftwareSync>
        <NeedTwoGroupOutOfScopeUpdates>true</NeedTwoGroupOutOfScopeUpdates>
        <AlsoPerformRegularSync>true</AlsoPerformRegularSync>
        <ComputerSpec/>
        <ExtendedUpdateInfoParameters>
          <XmlUpdateFragmentTypes>
            <XmlUpdateFragmentType>Extended</XmlUpdateFragmentType>
            <XmlUpdateFragmentType>LocalizedProperties</XmlUpdateFragmentType>
          </XmlUpdateFragmentTypes>
          <Locales><string>en-US</string></Locales>
        </ExtendedUpdateInfoParameters>
        <ClientPreferredLanguages/>
        <ProductsParameters>
          <SyncCurrentVersionOnly>%s</SyncCurrentVersionOnly>
          <DeviceAttributes>%s</DeviceAttributes>
          <CallerAttributes>%s</CallerAttributes>
          <Products>%s</Products>
        </ProductsParameters>
      </parameters>
    </SyncUpdates>
  </s:Body>
</s:Envelope>`,
		msgID, ClientEndpoint,
		created, expires,
		deviceToken,
		cookieExpiration,
		encryptedData,
		installedIDs,
		syncCurrent,
		htmlEncode(deviceAttrs),
		htmlEncode(callerAttrs),
		htmlEncode(products),
	)
}

// BuildGetEUI2Envelope returns the SOAP XML body for a GetExtendedUpdateInfo2 request.
func BuildGetEUI2Envelope(now time.Time, deviceToken string, updateID string, revision int, deviceAttrs string) string {
	msgID := newMessageID()
	created := w3cTime(now)
	expires := w3cTime(now.Add(120 * time.Second))

	return fmt.Sprintf(`<s:Envelope xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetExtendedUpdateInfo2</a:Action>
    <a:MessageID>urn:uuid:%s</a:MessageID>
    <a:To s:mustUnderstand="1">%s</a:To>
    <o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <Timestamp xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
        <Created>%s</Created>
        <Expires>%s</Expires>
      </Timestamp>
      <wuws:WindowsUpdateTicketsToken wsu:id="ClientMSA" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd" xmlns:wuws="http://schemas.microsoft.com/msus/2014/10/WindowsUpdateAuthorization">
        <TicketType Name="MSA" Version="1.0" Policy="MBI_SSL">
          <Device>%s</Device>
        </TicketType>
      </wuws:WindowsUpdateTicketsToken>
    </o:Security>
  </s:Header>
  <s:Body>
    <GetExtendedUpdateInfo2 xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
      <updateIDs>
        <UpdateIdentity>
          <UpdateID>%s</UpdateID>
          <RevisionNumber>%d</RevisionNumber>
        </UpdateIdentity>
      </updateIDs>
      <infoTypes>
        <XmlUpdateFragmentType>FileUrl</XmlUpdateFragmentType>
        <XmlUpdateFragmentType>FileDecryption</XmlUpdateFragmentType>
        <XmlUpdateFragmentType>EsrpDecryptionInformation</XmlUpdateFragmentType>
        <XmlUpdateFragmentType>PiecesHashUrl</XmlUpdateFragmentType>
        <XmlUpdateFragmentType>BlockMapUrl</XmlUpdateFragmentType>
      </infoTypes>
      <deviceAttributes>%s</deviceAttributes>
    </GetExtendedUpdateInfo2>
  </s:Body>
</s:Envelope>`,
		msgID, ClientSecuredEndpoint,
		created, expires,
		deviceToken,
		updateID, revision,
		htmlEncode(deviceAttrs),
	)
}

// htmlEncode encodes & characters as &amp; in attribute values.
func htmlEncode(s string) string {
	return strings.ReplaceAll(s, "&", "&amp;")
}

// buildInstalledUpdateIDsXML returns the XML fragment of known non-leaf update
// IDs. These tell the Windows Update service which categories the client has
// already "installed", filtering results appropriately.
func buildInstalledUpdateIDsXML() string {
	var sb strings.Builder
	for _, id := range installedNonLeafUpdateIDs {
		fmt.Fprintf(&sb, "<int>%d</int>", id)
	}
	return sb.String()
}

// installedNonLeafUpdateIDs is the list of Windows Update category/detectoid IDs
// that the WU client claims to have "installed". This tells the service which
// product categories to use when selecting applicable updates.
//
// List sourced directly from the UUP dump PHP reference implementation
// (shared/requests.php, InstalledNonLeafUpdateIDs block).
var installedNonLeafUpdateIDs = []int{
	1, 2, 3, 10, 11, 17, 19,
	23110993, 23110994, 23110995, 23110996, 23110999, 23111000, 23111001,
	23111002, 23111003, 23111004,
	2359974, 2359977,
	24513870,
	28880263,
	296374060,
	30077688,
	30486944,
	316003061,
	326686062, 326686063,
	327065581,
	327072300, 327072305,
	327100345,
	5143990,
	5169043, 5169044, 5169047,
	59830006, 59830007, 59830008,
	60484010,
	62450018, 62450019, 62450020,
	69801474,
	8788830,
	8806526,
	9125350,
	9154769,
	98959022, 98959023, 98959024, 98959025, 98959026,
	105939029,
	105995585,
	106017178,
	107825194,
	10809856,
	117765322,
	129905029,
	130040030, 130040031, 130040032, 130040033,
	133399034,
	138372035, 138372036,
	139536037, 139536038, 139536039, 139536040,
	142045136,
	158941041, 158941042, 158941043, 158941044,
	159776047,
	160733048, 160733049, 160733050, 160733051, 160733055, 160733056,
	161870057, 161870058, 161870059,
}
