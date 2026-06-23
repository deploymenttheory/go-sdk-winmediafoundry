// Package soap provides Windows Update SOAP protocol utilities.
//
// HTTP execution has moved to the windowsuup/client Transport layer. This
// package now exposes only:
//   - Endpoint and SOAP action constants
//   - CookieManager for WU session cookie acquisition and caching
//   - Envelope builders (BuildSyncUpdatesEnvelope, BuildGetEUI2Envelope)
//   - Response parsers (ParseSyncUpdatesResponse, ParseFileURLs)
//   - Domain helpers (BuildDeviceAttributes, BuildProductsString, IsCookieError)
package soap

// Windows Update SOAP endpoint URLs.
const (
	// ClientEndpoint is used for GetCookie and SyncUpdates calls.
	ClientEndpoint = "https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx"

	// ClientSecuredEndpoint is used for GetExtendedUpdateInfo2 calls.
	ClientSecuredEndpoint = "https://fe3cr.delivery.mp.microsoft.com/ClientWebService/client.asmx/secured"
)

// Windows Update SOAP action URIs.
const (
	GetCookieAction   = "http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetCookie"
	SyncUpdatesAction = "http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/SyncUpdates"
	GetEUI2Action     = "http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetExtendedUpdateInfo2"
)

// UserAgent is the Windows Update client user agent string sent with every SOAP request.
const UserAgent = "Windows-Update-Agent/10.0.10011.16384 Client-Protocol/2.50"

// ContentType is the SOAP 1.2 content type.
const ContentType = "application/soap+xml; charset=utf-8"

// CallerAttrs is the WU caller attributes string embedded in SyncUpdates
// SOAP request bodies.
const CallerAttrs = "E:Profile=AUv2&Acquisition=1&Interactive=1&IsSeeker=1&SheddingAware=1&Id=MoUpdateOrchestrator"
