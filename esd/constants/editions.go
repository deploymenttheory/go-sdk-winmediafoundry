package constants

// Edition is the Windows edition name as it appears in update filenames.
// Used for file-level filtering when downloading a specific Windows variant.
type Edition string

const (
	// Client editions.
	EditionHome           Edition = "CORE"
	EditionHomeN          Edition = "COREN"
	EditionProfessional   Edition = "PROFESSIONAL"
	EditionProfessionalN  Edition = "PROFESSIONALN"
	EditionEnterprise     Edition = "ENTERPRISE"
	EditionEnterpriseN    Edition = "ENTERPRISEN"
	EditionEducation      Edition = "EDUCATION"
	EditionEducationN     Edition = "EDUCATIONN"
	EditionProWorkstation Edition = "PPIPRO"

	// Server editions.
	EditionServerStandard   Edition = "SERVERSTANDARD"
	EditionServerDatacenter Edition = "SERVERDATACENTER"

	// Special.
	EditionNeutral Edition = "" // language/edition-neutral files
)

// EditionDisplayName returns the human-readable name for an edition.
func EditionDisplayName(e Edition) string {
	names := map[Edition]string{
		EditionHome:             "Windows Home",
		EditionHomeN:            "Windows Home N",
		EditionProfessional:     "Windows Professional",
		EditionProfessionalN:    "Windows Professional N",
		EditionEnterprise:       "Windows Enterprise",
		EditionEnterpriseN:      "Windows Enterprise N",
		EditionEducation:        "Windows Education",
		EditionEducationN:       "Windows Education N",
		EditionProWorkstation:   "Windows Pro for Workstations",
		EditionServerStandard:   "Windows Server Standard",
		EditionServerDatacenter: "Windows Server Datacenter",
	}
	if name, ok := names[e]; ok {
		return name
	}
	return string(e)
}
