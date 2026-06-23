package wim

import (
	"encoding/xml"
	"fmt"
	"unicode/utf16"
)

// ImageInfo describes a single image within a WIM, parsed from the XML catalog.
type ImageInfo struct {
	Index            int
	Name             string
	Description      string
	DisplayName      string
	Flags            string
	Edition          string
	Architecture     string
	InstallationType string
	ProductName      string
	DirCount         int64
	FileCount        int64
	TotalBytes       int64
	Languages        []string
}

// Images returns the images described by the WIM's XML catalog.
func (w *WIM) Images() []ImageInfo { return w.images }

// XML returns the raw (UTF-8) XML catalog.
func (w *WIM) XML() string { return w.xmlUTF8 }

// xmlWIM and friends mirror the WIM XML catalog structure.
type xmlWIM struct {
	Images []xmlImage `xml:"IMAGE"`
}

type xmlImage struct {
	Index       int        `xml:"INDEX,attr"`
	Name        string     `xml:"NAME"`
	Description string     `xml:"DESCRIPTION"`
	DisplayName string     `xml:"DISPLAYNAME"`
	Flags       string     `xml:"FLAGS"`
	DirCount    int64      `xml:"DIRCOUNT"`
	FileCount   int64      `xml:"FILECOUNT"`
	TotalBytes  int64      `xml:"TOTALBYTES"`
	Windows     xmlWindows `xml:"WINDOWS"`
}

type xmlWindows struct {
	Arch             int          `xml:"ARCH"`
	EditionID        string       `xml:"EDITIONID"`
	InstallationType string       `xml:"INSTALLATIONTYPE"`
	ProductName      string       `xml:"PRODUCTNAME"`
	Languages        xmlLanguages `xml:"LANGUAGES"`
}

type xmlLanguages struct {
	Language []string `xml:"LANGUAGE"`
}

// loadXML reads and parses the (uncompressed) XML catalog resource.
func (w *WIM) loadXML(_ int64) error {
	rd := w.hdr.XMLData
	if rd.CompressedSize == 0 {
		return nil
	}
	if rd.compressed() {
		return fmt.Errorf("wim: compressed XML catalog not yet supported")
	}
	raw, err := w.readResourceRaw(rd)
	if err != nil {
		return err
	}

	w.xmlUTF8 = decodeUTF16(raw)
	var doc xmlWIM
	if err := xml.Unmarshal([]byte(w.xmlUTF8), &doc); err != nil {
		return fmt.Errorf("wim: parse XML catalog: %w", err)
	}

	w.images = make([]ImageInfo, 0, len(doc.Images))
	for _, im := range doc.Images {
		w.images = append(w.images, ImageInfo{
			Index:            im.Index,
			Name:             im.Name,
			Description:      im.Description,
			DisplayName:      im.DisplayName,
			Flags:            im.Flags,
			Edition:          im.Windows.EditionID,
			Architecture:     archName(im.Windows.Arch),
			InstallationType: im.Windows.InstallationType,
			ProductName:      im.Windows.ProductName,
			DirCount:         im.DirCount,
			FileCount:        im.FileCount,
			TotalBytes:       im.TotalBytes,
			Languages:        im.Windows.Languages.Language,
		})
	}
	return nil
}

// archName maps a WIM PROCESSOR_ARCHITECTURE code to a name.
func archName(code int) string {
	switch code {
	case 0:
		return "x86"
	case 5:
		return "arm"
	case 6:
		return "ia64"
	case 9:
		return "x64"
	case 12:
		return "arm64"
	default:
		return fmt.Sprintf("arch(%d)", code)
	}
}

// decodeUTF16 converts WIM XML bytes (UTF-16LE, optionally BOM-prefixed) to a
// UTF-8 string. Non-UTF-16 input is returned unchanged.
func decodeUTF16(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:] // strip little-endian BOM
	}
	if len(b)%2 != 0 {
		return string(b)
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u16))
}
