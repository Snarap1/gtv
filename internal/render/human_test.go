package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

const humanFixture = `
{"e":"suiteStart","task":":m:test","key":":m:test/1","parent":null,"name":"Gradle Test Run :m:test","display":"Gradle Test Run :m:test","cls":""}
{"e":"suiteStart","task":":m:test","key":":m:test/1.1","parent":":m:test/1","name":"Gradle Test Executor 1","display":"Gradle Test Executor 1","cls":""}
{"e":"suiteStart","task":":m:test","key":":m:test/1.2","parent":":m:test/1.1","name":"com.mm.GtvProbeTest","display":"GtvProbeTest","cls":"com.mm.GtvProbeTest"}
{"e":"testStart","task":":m:test","key":":m:test/1.3","parent":":m:test/1.2","name":"plain","display":"plain works()","cls":"com.mm.GtvProbeTest"}
{"e":"testEnd","task":":m:test","key":":m:test/1.3","parent":":m:test/1.2","name":"plain","display":"plain works()","cls":"com.mm.GtvProbeTest","res":"SUCCESS","start":1000,"end":1010,"failures":[]}
{"e":"suiteStart","task":":m:test","key":":m:test/1.4","parent":":m:test/1.2","name":"Nested","display":"Nested","cls":"com.mm.GtvProbeTest$Nested"}
{"e":"testStart","task":":m:test","key":":m:test/1.5","parent":":m:test/1.4","name":"deep","display":"deep works()","cls":"com.mm.GtvProbeTest$Nested"}
{"e":"testEnd","task":":m:test","key":":m:test/1.5","parent":":m:test/1.4","name":"deep","display":"deep works()","cls":"com.mm.GtvProbeTest$Nested","res":"SUCCESS","start":1010,"end":1015,"failures":[]}
{"e":"suiteEnd","task":":m:test","key":":m:test/1.4","parent":":m:test/1.2","name":"Nested","display":"Nested","cls":"com.mm.GtvProbeTest$Nested","res":"SUCCESS","start":1010,"end":1015,"total":1,"ok":1,"failed":0,"skipped":0,"failures":[]}
{"e":"suiteStart","task":":m:test","key":":m:test/1.6","parent":":m:test/1.2","name":"paramCombos","display":"combos(int, int)","cls":"com.mm.GtvProbeTest"}
{"e":"testStart","task":":m:test","key":":m:test/1.7","parent":":m:test/1.6","name":"1","display":"a=1 -> b=1","cls":"com.mm.GtvProbeTest"}
{"e":"testEnd","task":":m:test","key":":m:test/1.7","parent":":m:test/1.6","name":"1","display":"a=1 -> b=1","cls":"com.mm.GtvProbeTest","res":"SUCCESS","start":1015,"end":1016,"failures":[]}
{"e":"testStart","task":":m:test","key":":m:test/1.8","parent":":m:test/1.6","name":"2","display":"a=2 -> b=4","cls":"com.mm.GtvProbeTest"}
{"e":"testEnd","task":":m:test","key":":m:test/1.8","parent":":m:test/1.6","name":"2","display":"a=2 -> b=4","cls":"com.mm.GtvProbeTest","res":"FAILURE","start":1016,"end":1017,"failures":[{"msg":"expected: <4> but was: <3>","cls":"org.opentest4j.AssertionFailedError","stack":"","assertion":true,"expected":"4","actual":"3"}]}
{"e":"testStart","task":":m:test","key":":m:test/1.9","parent":":m:test/1.6","name":"3","display":"a=3 -> b=9","cls":"com.mm.GtvProbeTest"}
{"e":"testEnd","task":":m:test","key":":m:test/1.9","parent":":m:test/1.6","name":"3","display":"a=3 -> b=9","cls":"com.mm.GtvProbeTest","res":"SUCCESS","start":1017,"end":1018,"failures":[]}
{"e":"suiteEnd","task":":m:test","key":":m:test/1.6","parent":":m:test/1.2","name":"paramCombos","display":"combos(int, int)","cls":"com.mm.GtvProbeTest","res":"FAILURE","start":1015,"end":1018,"total":3,"ok":2,"failed":1,"skipped":0,"failures":[]}
{"e":"suiteEnd","task":":m:test","key":":m:test/1.2","parent":":m:test/1.1","name":"com.mm.GtvProbeTest","display":"GtvProbeTest","cls":"com.mm.GtvProbeTest","res":"FAILURE","start":1000,"end":1018,"total":5,"ok":4,"failed":1,"skipped":0,"failures":[]}
{"e":"suiteEnd","task":":m:test","key":":m:test/1.1","parent":":m:test/1","name":"Gradle Test Executor 1","display":"Gradle Test Executor 1","cls":"","res":"FAILURE","start":1000,"end":1018,"total":5,"ok":4,"failed":1,"skipped":0,"failures":[]}
{"e":"suiteEnd","task":":m:test","key":":m:test/1","parent":null,"name":"Gradle Test Run :m:test","display":"Gradle Test Run :m:test","cls":"","res":"FAILURE","start":1000,"end":1018,"total":5,"ok":4,"failed":1,"skipped":0,"failures":[]}
`

func buildFixture(t *testing.T) *model.Tree {
	t.Helper()
	tree := model.New()
	for _, line := range strings.Split(strings.TrimSpace(humanFixture), "\n") {
		e, ok := event.Decode([]byte(line))
		if !ok {
			t.Fatalf("bad fixture line: %s", line)
		}
		tree.Apply(e)
	}
	return tree
}

func TestHumanRendersTree(t *testing.T) {
	tree := buildFixture(t)
	var buf bytes.Buffer
	Human(&buf, tree, HumanOptions{Color: false, Options: DefaultOptions()})
	out := buf.String()

	for _, want := range []string{
		"FAIL",
		"GtvProbeTest",
		"├─ ✓ plain works",
		"├─ Nested",
		"✓ deep works",
		"combos",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestHumanCollapsesPassingParamGroup(t *testing.T) {
	tree := model.New()
	for _, line := range strings.Split(strings.TrimSpace(humanFixture), "\n") {
		e, ok := event.Decode([]byte(line))
		if !ok {
			t.Fatalf("bad fixture line: %s", line)
		}

		if e.Key == ":m:test/1.8" && e.E == event.TestEnd {
			e.Res = event.Success
			e.Failures = nil
		}
		if e.Key == ":m:test/1.6" && e.E == event.SuiteEnd {
			e.Res, e.Failed, e.Ok = event.Success, 0, 3
		}
		if e.Key == ":m:test/1.2" && e.E == event.SuiteEnd {
			e.Res, e.Failed, e.Ok = event.Success, 0, 5
		}
		for _, ancestor := range []string{":m:test/1.1", ":m:test/1"} {
			if e.Key == ancestor && e.E == event.SuiteEnd {
				e.Res, e.Failed, e.Ok = event.Success, 0, 5
			}
		}
		tree.Apply(e)
	}

	var buf bytes.Buffer
	Human(&buf, tree, HumanOptions{Color: false, Options: DefaultOptions()})
	out := buf.String()

	if !strings.Contains(out, "3/3 ok") {
		t.Errorf("expected collapsed param group line, got:\n%s", out)
	}
	if strings.Contains(out, "a=1 -> b=1") {
		t.Errorf("passing invocations should stay collapsed, got:\n%s", out)
	}
}

func TestHumanExpandsOnlyFailedInvocations(t *testing.T) {
	tree := buildFixture(t)
	var buf bytes.Buffer
	Human(&buf, tree, HumanOptions{Color: false, Options: DefaultOptions()})
	out := buf.String()

	if !strings.Contains(out, "a=2 -> b=4") {
		t.Errorf("failed invocation should be expanded, got:\n%s", out)
	}
	if strings.Contains(out, "a=1 -> b=1") || strings.Contains(out, "a=3 -> b=9") {
		t.Errorf("passing invocations should stay collapsed, got:\n%s", out)
	}
	if !strings.Contains(out, "+2 ok") {
		t.Errorf("expected a collapsed-count line for the 2 passing invocations, got:\n%s", out)
	}
}

func TestColorizeNoop(t *testing.T) {
	if colorize(false, ansiRed, "x") != "x" {
		t.Fatal("colorize with color off must not add codes")
	}
	if got := colorize(true, ansiRed, "x"); !strings.HasPrefix(got, ansiRed) || !strings.HasSuffix(got, ansiReset) {
		t.Fatalf("colorize with color on = %q", got)
	}
}
