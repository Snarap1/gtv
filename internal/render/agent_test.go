package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

const twoFailuresFixture = `
{"e":"testEnd","task":":m:test","key":":m:test/1","parent":":m:test/r","name":"a","display":"a()","cls":"C","res":"FAILURE","start":1,"end":2,"failures":[{"msg":"boom","cls":"E","stack":"","assertion":true}]}
{"e":"testEnd","task":":m:test","key":":m:test/2","parent":":m:test/r","name":"b","display":"b()","cls":"C","res":"FAILURE","start":2,"end":3,"failures":[{"msg":"bam","cls":"E","stack":"","assertion":true}]}
`

func buildTwoFailures(t *testing.T) *model.Tree {
	t.Helper()
	tree := model.New()
	for _, line := range strings.Split(strings.TrimSpace(twoFailuresFixture), "\n") {
		e, ok := event.Decode([]byte(line))
		if !ok {
			t.Fatalf("bad fixture line: %s", line)
		}
		tree.Apply(e)
	}
	return tree
}

func TestAgentHintsHowToSeeAllFailures(t *testing.T) {
	var buf bytes.Buffer
	Agent(&buf, buildTwoFailures(t), Options{MaxFailures: 1, Target: ":m:test"})
	out := buf.String()

	for _, want := range []string{
		"+1 more failures",
		"hint: gtv :m:test --max-fail 0 shows all 2 failures",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestAgentOmitsHintWithoutTarget(t *testing.T) {
	var buf bytes.Buffer
	Agent(&buf, buildTwoFailures(t), Options{MaxFailures: 1})
	if strings.Contains(buf.String(), "hint:") {
		t.Errorf("hint printed without Target set:\n%s", buf.String())
	}
}

func TestAgentGreenRunHasNoHint(t *testing.T) {
	tree := model.New()
	e, ok := event.Decode([]byte(`{"e":"testEnd","task":":m:test","key":":m:test/1","parent":":m:test/r","name":"a","display":"a()","cls":"C","res":"SUCCESS","start":1,"end":2,"failures":[]}`))
	if !ok {
		t.Fatal("bad fixture line")
	}
	tree.Apply(e)

	var buf bytes.Buffer
	Agent(&buf, tree, Options{MaxFailures: 1, Target: ":m:test"})
	out := buf.String()
	if !strings.HasPrefix(out, "PASS") {
		t.Errorf("expected PASS, got:\n%s", out)
	}
	if strings.Contains(out, "hint:") {
		t.Errorf("hint on a fully reported run:\n%s", out)
	}
}
