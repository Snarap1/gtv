package lastresults

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirMapsTaskToResultsDir(t *testing.T) {
	got := Dir("/root", ":a:b:test")
	want := filepath.Join("/root", "a", "b", "build", "test-results", "test")
	if got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
}

func TestDirBareTask(t *testing.T) {
	got := Dir("/root", ":test")
	want := filepath.Join("/root", "build", "test-results", "test")
	if got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
}

const flatXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.FooTest" tests="2" failures="1" skipped="0" time="1.250">
  <testcase name="should pass" classname="com.example.FooTest" time="0.500"/>
  <testcase name="should fail" classname="com.example.FooTest" time="0.750">
    <failure message="expected: &lt;1&gt; but was: &lt;2&gt;" type="org.opentest4j.AssertionFailedError">org.opentest4j.AssertionFailedError: expected: &lt;1&gt; but was: &lt;2&gt;
	at com.example.FooTest.should fail(FooTest.java:20)
</failure>
  </testcase>
</testsuite>
`

func TestLoadFlatClass(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "TEST-com.example.FooTest.xml", flatXML)

	tree, err := Load(dir, ":moduleA:test", "")
	if err != nil {
		t.Fatal(err)
	}
	c := tree.Counts()
	if c.Total != 2 || c.Ok != 1 || c.Failed != 1 {
		t.Fatalf("counts = %+v", c)
	}

	suites := tree.Suites()
	if len(suites) != 1 || suites[0].Label() != "FooTest" {
		t.Fatalf("suites = %+v", suites)
	}

	for _, n := range tree.Leaves() {
		if n.Res == event.Failure {
			if len(n.Failures) != 1 {
				t.Fatalf("failures = %+v", n.Failures)
			}
			f := n.Failures[0]
			if f.Cls != "org.opentest4j.AssertionFailedError" {
				t.Errorf("failure class = %q", f.Cls)
			}
			if f.Stack == "" {
				t.Errorf("failure stack is empty")
			}
		}
	}
}

const nestedXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.OuterTest" tests="2" failures="0" skipped="1" time="0.900">
  <testcase name="outer case" classname="com.example.OuterTest" time="0.100"/>
  <testcase name="nested case" classname="com.example.OuterTest$InnerNested" time="0.300"/>
  <testcase name="skipped case" classname="com.example.OuterTest$InnerNested" time="0.000">
    <skipped message="disabled"/>
  </testcase>
</testsuite>
`

func TestLoadRestoresNestedHierarchy(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "TEST-com.example.OuterTest.xml", nestedXML)

	tree, err := Load(dir, ":test", "")
	if err != nil {
		t.Fatal(err)
	}

	suites := tree.Suites()
	if len(suites) != 1 {
		t.Fatalf("suites = %d, want 1", len(suites))
	}
	outer := suites[0]
	if outer.Label() != "OuterTest" {
		t.Fatalf("outer label = %q", outer.Label())
	}
	if outer.Total != 3 || outer.Ok != 2 || outer.Skipped != 1 {
		t.Fatalf("outer aggregate = total=%d ok=%d skipped=%d", outer.Total, outer.Ok, outer.Skipped)
	}

	found := false
	for _, c := range outer.Children {
		if !c.IsTest && c.Label() == "InnerNested" {
			found = true
			if c.Total != 2 || c.Ok != 1 || c.Skipped != 1 {
				t.Fatalf("nested aggregate = total=%d ok=%d skipped=%d", c.Total, c.Ok, c.Skipped)
			}
		}
	}
	if !found {
		t.Fatal("InnerNested suite not found among OuterTest's children")
	}
}

func TestLoadFilterByClass(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "TEST-com.example.FooTest.xml", flatXML)
	write(t, dir, "TEST-com.example.OuterTest.xml", nestedXML)

	tree, err := Load(dir, ":test", "com.example.FooTest")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Suites()) != 1 || tree.Suites()[0].Label() != "FooTest" {
		t.Fatalf("suites = %+v, want only FooTest", tree.Suites())
	}
}

func TestLoadFilterByMethod(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "TEST-com.example.FooTest.xml", flatXML)

	tree, err := Load(dir, ":test", "com.example.FooTest.should pass")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Counts().Total != 2 {
		t.Fatalf("counts = %+v, want the whole class loaded (filtering to a method happens in Gradle's --tests, not here)", tree.Counts())
	}
}

func TestLoadNoResultsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := Load(dir, ":test", "")
	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("err = %v, want ErrNoResults", err)
	}
}

func TestLoadNoMatchingClass(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "TEST-com.example.FooTest.xml", flatXML)

	_, err := Load(dir, ":test", "com.example.NoSuchTest")
	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("err = %v, want ErrNoResults", err)
	}
}
