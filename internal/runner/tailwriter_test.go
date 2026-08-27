package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
)

func TestTailWriterKeepsEverythingUnderLimit(t *testing.T) {
	var w tailWriter
	w.Write([]byte("hello "))
	w.Write([]byte("world"))
	if got := w.String(); got != "hello world" {
		t.Fatalf("String = %q", got)
	}
	if w.Total != 11 {
		t.Fatalf("Total = %d, want 11", w.Total)
	}
}

func TestTailWriterTotalCountsDroppedBytes(t *testing.T) {
	var w tailWriter
	w.Write([]byte(strings.Repeat("a", maxGradleLog)))
	w.Write([]byte("TAIL"))
	want := int64(maxGradleLog + 4)
	if w.Total != want {
		t.Fatalf("Total = %d, want %d (dropped bytes still count)", w.Total, want)
	}
}

func TestTailWriterKeepsTailAndMarksLoss(t *testing.T) {
	var w tailWriter
	w.Write([]byte(strings.Repeat("a", maxGradleLog)))
	w.Write([]byte("TAIL"))

	got := w.String()
	if !strings.HasSuffix(got, "TAIL") {
		t.Error("most recent output must survive")
	}
	if !strings.Contains(got, "earlier output dropped") {
		t.Error("dropped output must be disclosed, not silently lost")
	}
	if len(got)-len("[…earlier output dropped…]\n") > maxGradleLog {
		t.Errorf("retained %d bytes, limit is %d", len(got), maxGradleLog)
	}
}

func TestTailWriterHandlesOversizedSingleWrite(t *testing.T) {
	var w tailWriter
	n, err := w.Write([]byte(strings.Repeat("b", maxGradleLog*2)))
	if n != maxGradleLog*2 || err != nil {
		t.Fatalf("Write = %d, %v — must report the full length it accepted", n, err)
	}
	if !strings.HasSuffix(w.String(), "b") || len(w.String()) > maxGradleLog+64 {
		t.Errorf("retained %d bytes", len(w.String()))
	}
}

func TestTailEventsRejectsMalformedNDJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	err := tailEvents(path, stop, make(chan event.Event, 1))
	if err == nil || !strings.Contains(err.Error(), "malformed event stream") {
		t.Fatalf("tailEvents error = %v, want malformed event stream", err)
	}
}

func TestTailEventsRejectsUnterminatedFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(path, []byte(`{"e":"testStart"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	err := tailEvents(path, stop, make(chan event.Event, 1))
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("tailEvents error = %v, want unterminated final line", err)
	}
}
