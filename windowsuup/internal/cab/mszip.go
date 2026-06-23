package cab

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
)

var errMSZIPSignature = errors.New("cab: MSZIP block missing CK signature")

// mszipDecompress decodes MSZIP-compressed CFDATA blocks. Each block is a
// "CK"-prefixed raw DEFLATE stream whose dictionary is the previous block's
// uncompressed output (up to the last 32 KiB), per [MS-MCI]. Each block is read
// to its exact known uncompressed size, bounding decompression.
func mszipDecompress(blocks [][]byte, uncompSizes []int) ([]byte, error) {
	total := 0
	for _, n := range uncompSizes {
		total += n
	}
	out := make([]byte, 0, total)

	for i, b := range blocks {
		if len(b) < 2 || b[0] != 'C' || b[1] != 'K' {
			return nil, errMSZIPSignature
		}
		var dict []byte
		if len(out) > 0 {
			dict = out[max(0, len(out)-32768):]
		}
		fr := flate.NewReaderDict(bytes.NewReader(b[2:]), dict)
		buf := bytes.NewBuffer(make([]byte, 0, uncompSizes[i]))
		if _, err := io.CopyN(buf, fr, int64(uncompSizes[i])); err != nil {
			_ = fr.Close()
			return nil, fmt.Errorf("cab: MSZIP block %d: %w", i, err)
		}
		_ = fr.Close()
		out = append(out, buf.Bytes()...)
	}
	return out, nil
}
