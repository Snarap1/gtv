package render

import (
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
)

// Captured verbatim from Gradle 9.2 running a Kotlin/JUnit5 test; note the
// method name contains spaces, which a naive frame regex would miss.
const kotlinStack = `java.lang.IllegalStateException: boom from probe
	at com.example.service.GtvProbeTest$FailureShapes.helperThrows(GtvProbeTest.kt:52)
	at com.example.service.GtvProbeTest$FailureShapes.should fail with exception(GtvProbeTest.kt:38)
	at java.base/java.lang.reflect.Method.invoke(Method.java:580)
	at java.base/java.util.ArrayList.forEach(ArrayList.java:1596)`

func TestFramesKeepsUserCodeOnly(t *testing.T) {
	got := Frames(kotlinStack, 3)
	want := []string{"GtvProbeTest.kt:52", "GtvProbeTest.kt:38"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Frames = %v, want %v", got, want)
	}
}

func TestFramesRespectsMax(t *testing.T) {
	if got := Frames(kotlinStack, 1); len(got) != 1 {
		t.Fatalf("Frames with max 1 = %v", got)
	}
}

func TestHeadlinePrefixesClassOnce(t *testing.T) {
	f := event.Fail{Cls: "java.lang.AssertionError", Msg: "\nExpected size: 5 but was: 3 in:\n[1, 2, 3]"}
	got := Headline(f, 6)
	want := []string{"AssertionError: Expected size: 5 but was: 3 in:", "[1, 2, 3]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
}

func TestHeadlineTruncates(t *testing.T) {
	f := event.Fail{Cls: "X", Msg: "a\nb\nc\nd"}
	got := Headline(f, 2)
	if len(got) != 3 || got[2] != "…" {
		t.Fatalf("Headline = %q", got)
	}
}

func TestWantsDiffSuppressesRedundantPair(t *testing.T) {
	redundant := event.Fail{Msg: "expected: <42> but was: <7>", Expected: "42", Actual: "7"}
	if wantsDiff(redundant) {
		t.Error("expected/actual already in message, should not be repeated")
	}
	useful := event.Fail{Msg: "arrays differ", Expected: "[1, 2]", Actual: "[1, 3]"}
	if !wantsDiff(useful) {
		t.Error("expected/actual carry new information, should be printed")
	}
}
