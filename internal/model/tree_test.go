package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
)

func TestTrimSignature(t *testing.T) {
	cases := map[string]string{
		"should pass()": "should pass",
		"should coerce limit and calculate offset correctly(int, int, int, int)": "should coerce limit and calculate offset correctly",

		"page=1 size=50 -> expectedLimit=50 expectedOffset=0": "page=1 size=50 -> expectedLimit=50 expectedOffset=0",
		"UserQueryService":     "UserQueryService",
		"search() golden path": "search() golden path",
		"()":                   "()",
	}
	for in, want := range cases {
		if got := trimSignature(in); got != want {
			t.Errorf("trimSignature(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppendOutputBoundsMemory(t *testing.T) {
	n := &Node{}
	for i := 0; i < maxOutChunks*3; i++ {
		n.appendOutput("line")
	}
	if len(n.Out) != maxOutChunks {
		t.Fatalf("kept %d chunks, want %d", len(n.Out), maxOutChunks)
	}

	long := make([]byte, maxOutChunkLen*2)
	for i := range long {
		long[i] = 'x'
	}
	n.appendOutput(string(long))
	if last := n.Out[len(n.Out)-1]; len(last) > maxOutChunkLen+4 {
		t.Errorf("chunk kept %d bytes, limit is %d", len(last), maxOutChunkLen)
	}
}

func TestAppendOutputKeepsMostRecent(t *testing.T) {
	n := &Node{}
	for i := 0; i < maxOutChunks+5; i++ {
		n.appendOutput(string(rune('a' + i%26)))
	}
	want := string(rune('a' + (maxOutChunks+4)%26))
	if got := n.Out[len(n.Out)-1]; got != want {
		t.Errorf("last chunk = %q, want %q — the tail is what gets rendered", got, want)
	}
}

func TestTreeOutputHasGlobalLimit(t *testing.T) {
	tree := New()
	for i := 0; i < maxTreeOutBytes/maxOutChunkLen+2; i++ {
		key := fmt.Sprintf("test-%d", i)
		tree.Apply(event.Event{E: event.TestStart, Key: key})
		tree.Apply(event.Event{E: event.Output, Key: key, Msg: strings.Repeat("x", maxOutChunkLen)})
	}
	if tree.outBytes > maxTreeOutBytes {
		t.Fatalf("retained output = %d bytes, limit = %d", tree.outBytes, maxTreeOutBytes)
	}
}
