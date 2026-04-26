package constants

// SKU represents a Windows edition identified by its WU numeric SKU ID.
// Use the named variables (e.g. SKUPro) rather than constructing SKU directly.
type SKU struct {
	// Name is the human-readable edition name.
	Name string
	// ID is the numeric OSSkuId sent in SOAP device attributes.
	ID int
}

// Windows client SKUs.
var (
	SKUHome           = SKU{"Home", 1}
	SKUHomeN          = SKU{"Home N", 2}
	SKUPro            = SKU{"Professional", 48} // default
	SKUProN           = SKU{"Professional N", 49}
	SKUEnterprise     = SKU{"Enterprise", 4}
	SKUEnterpriseN    = SKU{"Enterprise N", 27}
	SKUEducation      = SKU{"Education", 121}
	SKUEducationN     = SKU{"Education N", 122}
	SKUProWorkstation = SKU{"Pro for Workstations", 161}
	SKUIoTEnterprise  = SKU{"IoT Enterprise", 188}
)

// Windows Server SKUs.
var (
	SKUServerStandard       = SKU{"Server Standard", 7}
	SKUServerDatacenter     = SKU{"Server Datacenter", 8}
	SKUServerStandardCore   = SKU{"Server Standard Core", 13}
	SKUServerDatacenterCore = SKU{"Server Datacenter Core", 12}
)
