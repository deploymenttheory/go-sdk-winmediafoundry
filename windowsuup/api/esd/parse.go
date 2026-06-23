package esd

import (
	"encoding/xml"
	"fmt"

	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
)

// xmlCatalog mirrors the structure of Microsoft's products.xml:
//
//	<MCT><Catalogs><Catalog><PublishedMedia><Files><File>...</File>...
type xmlCatalog struct {
	Files []xmlFile `xml:"Catalogs>Catalog>PublishedMedia>Files>File"`
}

type xmlFile struct {
	FileName     string `xml:"FileName"`
	LanguageCode string `xml:"LanguageCode"`
	Language     string `xml:"Language"`
	Edition      string `xml:"Edition"`
	Architecture string `xml:"Architecture"`
	Size         int64  `xml:"Size"`
	SHA1         string `xml:"Sha1"`
	FilePath     string `xml:"FilePath"`
}

// parseCatalog parses a products.xml document into an ESDCatalog.
func parseCatalog(data []byte) (*models.ESDCatalog, error) {
	var doc xmlCatalog
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal products.xml: %w", err)
	}

	images := make([]models.ESDImage, 0, len(doc.Files))
	for _, f := range doc.Files {
		images = append(images, models.ESDImage{
			FileName:     f.FileName,
			Edition:      f.Edition,
			Architecture: f.Architecture,
			LanguageCode: f.LanguageCode,
			Language:     f.Language,
			SizeBytes:    f.Size,
			SHA1:         f.SHA1,
			URL:          f.FilePath,
		})
	}
	return &models.ESDCatalog{Images: images}, nil
}
