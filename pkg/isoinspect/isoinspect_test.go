package isoinspect

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/iso"
)

func writeFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeEfisys returns a blob that the validator recognises as a FAT EFI image:
// a FAT-style jump at the start and the 0x55AA signature at offset 510.
func fakeEfisys(size int) []byte {
	b := make([]byte, size)
	b[0], b[1], b[2] = 0xEB, 0x3C, 0x90
	b[510], b[511] = 0x55, 0xAA
	return b
}

// buildTestISO masters a small Windows-shaped UDF+El Torito ISO. withBIOS builds
// x64-style dual-boot media (efi/boot/bootx64.efi + boot/etfsboot.com), which the
// builder keeps a BIOS entry for; otherwise it builds ARM64 UEFI-only media
// (efi/boot/bootaa64.efi), for which the builder must drop any BIOS entry.
func buildTestISO(t *testing.T, withBIOS bool) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "efi/microsoft/boot/efisys.bin", fakeEfisys(4096))
	writeFile(t, root, "sources/boot.wim", bytes.Repeat([]byte{0x11}, 9000))
	writeFile(t, root, "sources/install.wim", bytes.Repeat([]byte{0x22}, 20000))
	writeFile(t, root, "setup.exe", []byte("setup"))
	if withBIOS {
		writeFile(t, root, "efi/boot/bootx64.efi", []byte("BOOTX64"))
		// An x86 boot sector starts with CLI (0xFA); make it look like etfsboot.com.
		etf := make([]byte, 2048)
		etf[0] = 0xFA
		writeFile(t, root, "boot/etfsboot.com", etf)
	} else {
		writeFile(t, root, "efi/boot/bootaa64.efi", []byte("BOOTAA64"))
	}

	out := filepath.Join(t.TempDir(), "win.iso")
	if err := iso.BuildWindowsUDF(root, out, "WIN_TEST"); err != nil {
		t.Fatalf("BuildWindowsUDF: %v", err)
	}
	return out
}

// TestARM64MediaIsUEFIOnly verifies the builder drops the stray x86 BIOS entry
// for ARM64 media (efi/boot/bootaa64.efi present) even when boot/etfsboot.com is
// in the tree, producing a UEFI-platform default entry like Microsoft's media.
func TestARM64MediaIsUEFIOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "efi/microsoft/boot/efisys.bin", fakeEfisys(4096))
	writeFile(t, root, "efi/boot/bootaa64.efi", []byte("BOOTAA64"))
	writeFile(t, root, "sources/boot.wim", bytes.Repeat([]byte{0x11}, 9000))
	etf := make([]byte, 2048)
	etf[0] = 0xFA
	writeFile(t, root, "boot/etfsboot.com", etf) // present, but must be ignored

	out := filepath.Join(t.TempDir(), "win.iso")
	if err := iso.BuildWindowsUDF(root, out, "CCCOMA_ARM64FRE"); err != nil {
		t.Fatal(err)
	}
	rep, err := Inspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ElTorito.BIOS != nil {
		t.Error("ARM64 media must not carry a BIOS El Torito entry")
	}
	if rep.ElTorito.ValidationPlatform != platformUEFI {
		t.Errorf("validation platform = %#02x, want 0xEF", rep.ElTorito.ValidationPlatform)
	}
	if !rep.OK() {
		t.Errorf("expected a clean ARM64 ISO, got:\n%s", rep.Summary())
	}
}

func TestInspectRoundTripUEFIOnly(t *testing.T) {
	out := buildTestISO(t, false)
	rep, err := Inspect(out)
	if err != nil {
		t.Fatal(err)
	}

	if rep.ElTorito == nil || rep.ElTorito.UEFI == nil {
		t.Fatal("no UEFI El Torito entry detected")
	}
	if !rep.ElTorito.UEFI.ImageIsFAT {
		t.Error("UEFI boot image not recognised as FAT")
	}
	if rep.UDF == nil || !rep.UDF.Present {
		t.Fatal("UDF not detected")
	}

	// The small files must be found and carry no allocation-descriptor errors.
	if !rep.OK() {
		t.Errorf("expected a clean UEFI-only ISO, got issues:\n%s", rep.Summary())
	}
	if !hasFile(rep.UDF.Files, "sources/boot.wim") || !hasFile(rep.UDF.Files, "sources/install.wim") {
		t.Errorf("expected sources/boot.wim and install.wim in UDF tree, got %v", rep.UDF.Files)
	}
}

func TestInspectFlagsBIOSEntry(t *testing.T) {
	out := buildTestISO(t, true)
	rep, err := Inspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ElTorito.BIOS == nil {
		t.Fatal("expected a BIOS boot entry")
	}
	if len(rep.Warnings()) == 0 {
		t.Error("expected an El Torito warning about the BIOS entry")
	}
	// UDF is still fine, so there must be no errors.
	if len(rep.Errors()) != 0 {
		t.Errorf("unexpected errors: %v", rep.Errors())
	}
}

func TestSetElToritoUEFIOnly(t *testing.T) {
	out := buildTestISO(t, true)
	if err := SetElToritoUEFIOnly(out); err != nil {
		t.Fatalf("SetElToritoUEFIOnly: %v", err)
	}
	rep, err := Inspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ElTorito.ValidationPlatform != platformUEFI {
		t.Errorf("validation platform = %#02x, want 0xEF", rep.ElTorito.ValidationPlatform)
	}
	if rep.ElTorito.BIOS != nil {
		t.Error("BIOS entry should be gone after SetElToritoUEFIOnly")
	}
	if rep.ElTorito.UEFI == nil || !rep.ElTorito.UEFI.ImageIsFAT {
		t.Error("UEFI entry must survive as the default")
	}
}

func TestExtractElToritoEFIImage(t *testing.T) {
	out := buildTestISO(t, false)
	dest := filepath.Join(t.TempDir(), "efisys.img")
	if err := ExtractElToritoEFIImage(out, dest); err != nil {
		t.Fatalf("ExtractElToritoEFIImage: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 512 || got[0] != 0xEB || got[510] != 0x55 || got[511] != 0xAA {
		t.Errorf("extracted EFI image does not look like the FAT efisys (len=%d)", len(got))
	}
}

// TestValidateFileExtents exercises the headline allocation-descriptor check
// directly, including the exact overflow that broke boot.wim.
func TestValidateFileExtents(t *testing.T) {
	const bootWIM = 1496060854

	t.Run("single_overflowing_ad_is_rejected", func(t *testing.T) {
		w := &udfWalker{r: &Report{}}
		// One short_ad with the length wrapped into the type bits — what the old
		// writer produced for a 1.5 GiB file.
		w.validateFileExtents("sources/boot.wim", bootWIM, []adExtent{
			{length: bootWIM & 0x3FFFFFFF, typ: bootWIM >> 30, block: 100},
		}, false)
		if len(w.r.Errors()) == 0 {
			t.Fatal("expected errors for a single overflowing allocation descriptor")
		}
	})

	t.Run("correct_multi_extent_passes", func(t *testing.T) {
		w := &udfWalker{r: &Report{}}
		w.validateFileExtents("sources/boot.wim", bootWIM, []adExtent{
			{length: maxShortADExtent, typ: 0, block: 100},
			{length: bootWIM - maxShortADExtent, typ: 0, block: 100 + maxShortADExtent/sectorSize},
		}, false)
		if len(w.r.Issues) != 0 {
			t.Fatalf("expected no issues for correct extents, got %v", w.r.Issues)
		}
	})

	t.Run("length_mismatch_is_rejected", func(t *testing.T) {
		w := &udfWalker{r: &Report{}}
		w.validateFileExtents("f", 1000, []adExtent{{length: 900, typ: 0, block: 5}}, false)
		if len(w.r.Errors()) == 0 {
			t.Fatal("expected an error when extents do not sum to the info length")
		}
	})
}

func hasFile(files []UDFFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
