// Package progress_counter provides a terminal progress bar for long-running
// operations — streaming downloads (via Reader) and large file writes such as
// ISO/WIM assembly (via WriteSeeker). The action verb ("Downloading",
// "Building", …) is configurable so the same bar serves any slow process.
package progress_counter

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

// defaultLabel is the action verb used when none is given.
const defaultLabel = "Downloading"

// minRenderInterval throttles redraws so the bar never becomes the bottleneck
// for callers that report progress on every (possibly tiny) write — e.g. an
// unbuffered WIM writer issuing thousands of small writes. The final update
// (written >= total) always renders.
const minRenderInterval = 100 * time.Millisecond

// Bar writes a progress bar to w (defaults to os.Stderr when nil). Create one
// per operation with New (or NewWithLabel), then either pass Bar.Callback to a
// progress callback, wrap a reader with Bar.Reader, or wrap a write-seeker with
// Bar.WriteSeeker.
//
// Example output (overwrites the same terminal line):
//
//	Building install.wim  [████████████░░░░░░░░░░░░░░░░]  1.2 GB / 4.0 GB  30.2%  45.3 MB/s  ETA 1m02s
type Bar struct {
	w        io.Writer
	label    string
	mu       sync.Mutex
	done     bool
	lastDraw time.Time
}

// New creates a Bar that writes to w with the default "Downloading" label. Pass
// nil to write to os.Stderr.
func New(w io.Writer) *Bar {
	return NewWithLabel(w, defaultLabel)
}

// NewWithLabel creates a Bar whose action verb is label (e.g. "Building"). An
// empty label falls back to the default.
func NewWithLabel(w io.Writer, label string) *Bar {
	if w == nil {
		w = os.Stderr
	}
	if label == "" {
		label = defaultLabel
	}
	return &Bar{w: w, label: label}
}

// Callback renders one progress update for item with the running byte counts.
// It matches the download progress-callback shape, so it can be passed directly
// where such a callback is expected.
func (b *Bar) Callback(item string, written, total int64, elapsed time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Throttle redraws so a high-frequency caller isn't slowed by terminal I/O;
	// always render the final update so the bar finishes at 100%.
	final := total > 0 && written >= total
	now := time.Now()
	if !final && now.Sub(b.lastDraw) < minRenderInterval {
		return
	}
	b.lastDraw = now

	if total <= 0 {
		// Unknown size: show bytes and speed only.
		writtenMB := float64(written) / (1024 * 1024)
		var speed float64
		if elapsed > 0 {
			speed = float64(written) / elapsed.Seconds() / (1024 * 1024)
		}
		fmt.Fprintf(b.w, "\r%s %-40s  %8.1f MB  %6.1f MB/s",
			b.label, truncate(item, 40), writtenMB, speed)
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

	fmt.Fprintf(b.w, "\r%s %-40s  [%s]  %7.1f MB / %.1f MB  %5.1f%%%s%s",
		b.label, truncate(item, 40), bar, writtenMB, totalMB, pct*100, speedStr, etaStr)

	if written >= total && !b.done {
		b.done = true
		fmt.Fprintln(b.w)
	}
}

// Reader wraps src in a counting reader that reports progress as bytes flow
// through. total should be the expected size (0 if unknown).
func (b *Bar) Reader(item string, src io.Reader, total int64) io.Reader {
	return &countingReader{src: src, item: item, total: total, bar: b, start: time.Now()}
}

// WriteSeeker wraps dst in a counting write-seeker that reports progress as
// bytes are written. It is suitable for instrumenting large file assembly (e.g.
// a WIM/ISO writer that needs Seek). total should be the expected output size
// (0 if unknown). Bytes rewritten after a Seek are still counted, so progress
// is an estimate.
func (b *Bar) WriteSeeker(item string, dst io.WriteSeeker, total int64) io.WriteSeeker {
	return &countingWriteSeeker{dst: dst, item: item, total: total, bar: b, start: time.Now()}
}

// countingReader wraps an io.Reader and reports progress on each read.
type countingReader struct {
	src     io.Reader
	item    string
	total   int64
	written atomic.Int64
	bar     *Bar
	start   time.Time
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		written := r.written.Add(int64(n))
		r.bar.Callback(r.item, written, r.total, time.Since(r.start))
	}
	return n, err
}

// countingWriteSeeker wraps an io.WriteSeeker and reports progress on each
// write, delegating seeks unchanged.
type countingWriteSeeker struct {
	dst     io.WriteSeeker
	item    string
	total   int64
	written atomic.Int64
	bar     *Bar
	start   time.Time
}

func (w *countingWriteSeeker) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if n > 0 {
		written := w.written.Add(int64(n))
		w.bar.Callback(w.item, written, w.total, time.Since(w.start))
	}
	return n, err
}

func (w *countingWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	return w.dst.Seek(offset, whence)
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
