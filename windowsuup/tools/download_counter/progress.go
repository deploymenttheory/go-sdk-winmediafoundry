// Package download_counter provides a terminal progress bar for streaming file downloads.
package download_counter

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const barWidth = 30

// Bar writes a download progress bar to w (defaults to os.Stderr when nil).
// Call New once per download session, then pass Bar.Callback to WithProgressCallback,
// or wrap the response body reader with Bar.Reader.
//
// Example output (overwrites the same terminal line):
//
//	Downloading Windows11.0-26100.4061-amd64.esd  [████████████░░░░░░░░░░░░░░░░]  1.2 GB / 4.0 GB  30.2%  45.3 MB/s  ETA 1m02s
type Bar struct {
	w    io.Writer
	mu   sync.Mutex
	done bool
}

// New creates a new Bar that writes to w. Pass nil to write to os.Stderr.
func New(w io.Writer) *Bar {
	if w == nil {
		w = os.Stderr
	}
	return &Bar{w: w}
}

// Callback satisfies the DownloadProgressCallback signature.
// Pass this to WithProgressCallback when constructing download options.
func (b *Bar) Callback(fileName string, written, total int64, elapsed time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if total <= 0 {
		// Unknown size: show bytes and speed only.
		writtenMB := float64(written) / (1024 * 1024)
		var speed float64
		if elapsed > 0 {
			speed = float64(written) / elapsed.Seconds() / (1024 * 1024)
		}
		fmt.Fprintf(b.w, "\rDownloading %-40s  %8.1f MB  %6.1f MB/s",
			truncate(fileName, 40), writtenMB, speed)
		return
	}

	pct := float64(written) / float64(total)
	filled := min(int(pct*barWidth), barWidth)
	empty := barWidth - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	writtenMB := float64(written) / (1024 * 1024)
	totalMB := float64(total) / (1024 * 1024)

	var speedStr, etaStr string
	if elapsed > 0 && written > 0 {
		bytesPerSec := float64(written) / elapsed.Seconds()
		speedStr = fmt.Sprintf("%6.1f MB/s", bytesPerSec/(1024*1024))
		remaining := float64(total-written) / bytesPerSec
		if remaining > 0 {
			etaStr = fmt.Sprintf("  ETA %s", formatDuration(time.Duration(remaining)*time.Second))
		}
	}

	fmt.Fprintf(b.w, "\rDownloading %-40s  [%s]  %7.1f MB / %.1f MB  %5.1f%%%s%s",
		truncate(fileName, 40), bar, writtenMB, totalMB, pct*100, speedStr, etaStr)

	if written >= total && !b.done {
		b.done = true
		fmt.Fprintln(b.w)
	}
}

// Reader wraps src in a counting reader that calls b.Callback as bytes flow through.
// total should be file.SizeBytes (0 if unknown).
func (b *Bar) Reader(fileName string, src io.Reader, total int64) io.Reader {
	return &countingReader{
		src:      src,
		fileName: fileName,
		total:    total,
		bar:      b,
		start:    time.Now(),
	}
}

// countingReader wraps an io.Reader and reports progress on each read.
type countingReader struct {
	src      io.Reader
	fileName string
	total    int64
	written  atomic.Int64
	bar      *Bar
	start    time.Time
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		written := r.written.Add(int64(n))
		r.bar.Callback(r.fileName, written, r.total, time.Since(r.start))
	}
	return n, err
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// formatDuration formats a duration as "Xm Ys" or "Xs".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
