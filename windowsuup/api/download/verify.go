package download

import (
	"bytes"
	"crypto/sha1"  //nolint:gosec // SHA1 used for legacy hash comparison only, not cryptographic security
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
)

// VerifyResult describes the outcome of verifying a single downloaded file.
type VerifyResult struct {
	File   models.File
	OK     bool
	Reason string // empty when OK; describes failure otherwise
}

// VerifyFiles checks each file in files against its on-disk counterpart in dir.
//
// Verification priority: SHA256 → SHA1 → size only (when no hashes are set).
// Hashes stored in models.File are base64-encoded (as returned by the SOAP API)
// and are decoded before comparison.
//
// Per-file failures are recorded in VerifyResult.Reason; the returned error
// is nil unless a systemic problem (e.g. unreadable directory) prevents
// verification from running at all.
func VerifyFiles(files []models.File, dir string) ([]VerifyResult, error) {
	results := make([]VerifyResult, 0, len(files))
	for _, f := range files {
		results = append(results, verifyOne(f, dir))
	}
	return results, nil
}

// verifyOne verifies a single file against its metadata.
func verifyOne(f models.File, dir string) VerifyResult {
	path := filepath.Join(dir, f.Name)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return VerifyResult{File: f, OK: false, Reason: "missing"}
		}
		return VerifyResult{File: f, OK: false, Reason: fmt.Sprintf("stat: %v", err)}
	}

	// Size check first — cheap and catches obvious corruption before hash I/O.
	if f.SizeBytes > 0 && info.Size() != f.SizeBytes {
		return VerifyResult{File: f, OK: false, Reason: fmt.Sprintf("size mismatch: got %d, want %d", info.Size(), f.SizeBytes)}
	}

	// SHA256 check (preferred).
	if f.SHA256 != "" {
		want, err := base64.StdEncoding.DecodeString(f.SHA256)
		if err != nil {
			return VerifyResult{File: f, OK: false, Reason: "invalid sha256 in metadata"}
		}
		actual256, _, err := computeHashes(path)
		if err != nil {
			return VerifyResult{File: f, OK: false, Reason: fmt.Sprintf("read: %v", err)}
		}
		if !bytes.Equal(actual256, want) {
			return VerifyResult{File: f, OK: false, Reason: "sha256 mismatch"}
		}
		return VerifyResult{File: f, OK: true}
	}

	// SHA1 fallback.
	if f.SHA1 != "" {
		want, err := base64.StdEncoding.DecodeString(f.SHA1)
		if err != nil {
			return VerifyResult{File: f, OK: false, Reason: "invalid sha1 in metadata"}
		}
		_, actual1, err := computeHashes(path)
		if err != nil {
			return VerifyResult{File: f, OK: false, Reason: fmt.Sprintf("read: %v", err)}
		}
		if !bytes.Equal(actual1, want) {
			return VerifyResult{File: f, OK: false, Reason: "sha1 mismatch"}
		}
		return VerifyResult{File: f, OK: true}
	}

	// No hashes available — size match (or absence of size) is sufficient.
	return VerifyResult{File: f, OK: true}
}

// computeHashes reads the file at path once and returns both its SHA256 and
// SHA1 digests.
func computeHashes(path string) (sha256sum, sha1sum []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	h256 := sha256.New()
	h1 := sha1.New() //nolint:gosec
	if _, err := io.Copy(io.MultiWriter(h256, h1), f); err != nil {
		return nil, nil, err
	}
	return h256.Sum(nil), h1.Sum(nil), nil
}
