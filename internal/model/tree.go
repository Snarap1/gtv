// Package model turns a flat event stream into the suite/test tree that the
// renderers walk.
package model

import (
	"regexp"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
)

// Node is a suite or a single test. Suites nest: class -> @Nested class ->
// (for parameterized tests) method -> invocation.
type Node struct {
	Key     string
	Task    string
	Name    string
	Display string
	Cls     string

	Parent   *Node
	Children []*Node

	IsTest bool
	Res    string
	Start  int64
	End    int64

	Failures []event.Fail
	Assumed  *event.Fail
	Out      []string

	// Total, Ok, Failed, Skipped are Gradle's own aggregate counts for a suite,
	// covering every descendant test. Zero for a leaf test node.
	Total, Ok, Failed, Skipped int
}

// Scaffolding reports whether the node is one of Gradle's synthetic wrappers
// ("Gradle Test Run :x:test", "Gradle Test Executor 1") rather than real test code.
func (n *Node) Scaffolding() bool { return !n.IsTest && n.Cls == "" }

// Label is the human name: the JUnit display name with a method signature or
// empty argument list trimmed off.
func (n *Node) Label() string { return trimSignature(n.Display) }

// Duration in milliseconds.
func (n *Node) Duration() int64 {
	if n.End > n.Start {
		return n.End - n.Start
	}
	return 0
}

// Path is the chain of labels from the outermost real suite down to this node.
func (n *Node) Path() []string {
	var parts []string
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Scaffolding() {
			continue
		}
		parts = append([]string{cur.Label()}, parts...)
	}
	return parts
}

// Captured test output is only ever rendered as a short tail, so a node keeps a
// bounded window of it: a chatty test must not be able to exhaust memory.
const (
	maxOutChunks    = 200
	maxOutChunkLen  = 4096
	maxTreeOutBytes = 8 * 1024 * 1024
)

// appendOutput records a chunk of the test's stdout/stderr, keeping only the
// most recent window.
func (n *Node) appendOutput(chunk string) int {
	if len(chunk) > maxOutChunkLen {
		chunk = chunk[:maxOutChunkLen-len("…")] + "…"
	}
	before := outputBytes(n.Out)
	n.Out = append(n.Out, chunk)
	if len(n.Out) > maxOutChunks {
		n.Out = append(n.Out[:0], n.Out[len(n.Out)-maxOutChunks:]...)
	}
	return outputBytes(n.Out) - before
}

func outputBytes(chunks []string) int {
	var total int
	for _, chunk := range chunks {
		total += len(chunk)
	}
	return total
}

// Tree accumulates events into nodes.
type Tree struct {
	byKey    map[string]*Node
	roots    []*Node
	tasks    []string
	outBytes int
}

func New() *Tree { return &Tree{byKey: map[string]*Node{}} }

// Roots returns the top-level nodes in the order they were first seen.
func (t *Tree) Roots() []*Node { return t.roots }

// Tasks returns the Gradle task paths seen in the stream, in order.
func (t *Tree) Tasks() []string { return t.tasks }

// Suites returns the outermost non-scaffolding nodes — normally one per test class.
func (t *Tree) Suites() []*Node {
	var out []*Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Scaffolding() {
				walk(n.Children)
				continue
			}
			out = append(out, n)
		}
	}
	walk(t.roots)
	return out
}

// Leaves returns every test node, in the order the tree was built — that is,
// the order tests started, not the order they finished.
func (t *Tree) Leaves() []*Node {
	var out []*Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.IsTest {
				out = append(out, n)
			}
			walk(n.Children)
		}
	}
	walk(t.roots)
	return out
}

// Counts of finished tests.
type Counts struct{ Total, Ok, Failed, Skipped int }

// Counts tallies leaves that have a result.
func (t *Tree) Counts() Counts {
	var c Counts
	for _, n := range t.Leaves() {
		switch n.Res {
		case event.Success:
			c.Ok++
		case event.Failure:
			c.Failed++
		case event.Skipped:
			c.Skipped++
		default:
			continue
		}
		c.Total++
	}
	return c
}

// Duration is the wall time of the whole run, taken from the root suites.
func (t *Tree) Duration() int64 {
	var first, last int64
	for _, n := range t.roots {
		if n.Start > 0 && (first == 0 || n.Start < first) {
			first = n.Start
		}
		if n.End > last {
			last = n.End
		}
	}
	if last > first {
		return last - first
	}
	return 0
}

// Apply folds one event into the tree.
func (t *Tree) Apply(e event.Event) {
	switch e.E {
	case event.SuiteStart, event.TestStart:
		t.node(e, e.E == event.TestStart)
	case event.SuiteEnd, event.TestEnd:
		n := t.node(e, e.E == event.TestEnd)
		n.Res, n.Start, n.End = e.Res, e.Start, e.End
		n.Failures, n.Assumed = e.Failures, e.Assumed
		if e.E == event.TestEnd && e.Res != event.Failure {
			t.outBytes -= outputBytes(n.Out)
			n.Out = nil
		}
		if e.E == event.SuiteEnd {
			n.Total, n.Ok, n.Failed, n.Skipped = int(e.Total), int(e.Ok), int(e.Failed), int(e.SkipCnt)
		}
	case event.Output:
		if n, ok := t.byKey[e.Key]; ok {
			t.appendOutput(n, e.Msg)
		}
	}
	t.trackTask(e.Task)
}

func (t *Tree) appendOutput(n *Node, chunk string) {
	if t.outBytes >= maxTreeOutBytes {
		return
	}
	if remaining := maxTreeOutBytes - t.outBytes; len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	t.outBytes += n.appendOutput(chunk)
}

func (t *Tree) node(e event.Event, isTest bool) *Node {
	if n, ok := t.byKey[e.Key]; ok {
		if isTest {
			n.IsTest = true
		}
		return n
	}
	n := &Node{Key: e.Key, Task: e.Task, Name: e.Name, Display: e.Display, Cls: e.Cls, IsTest: isTest}
	t.byKey[e.Key] = n

	// Suites always start before their children, so the parent exists by now;
	// if it somehow does not, treat this node as a root.
	if p, ok := t.byKey[e.Parent]; e.Parent != "" && ok {
		n.Parent = p
		p.Children = append(p.Children, n)
	} else {
		t.roots = append(t.roots, n)
	}
	return n
}

func (t *Tree) trackTask(task string) {
	if task == "" {
		return
	}
	for _, existing := range t.tasks {
		if existing == task {
			return
		}
	}
	t.tasks = append(t.tasks, task)
}

// A trailing parenthesised group is a JVM method signature ("foo(int, int)") or
// an empty argument list, both noise. Parameterized invocation names look like
// "page=1 size=50 -> ..." and must survive.
var signature = regexp.MustCompile(`\(([\w.$<>\[\], ]*)\)$`)

func trimSignature(display string) string {
	m := signature.FindStringSubmatch(display)
	if m == nil {
		return display
	}
	for _, arg := range strings.Split(m[1], ",") {
		if arg = strings.TrimSpace(arg); arg != "" && strings.Contains(arg, " ") {
			return display
		}
	}
	trimmed := strings.TrimSpace(display[:len(display)-len(m[0])])
	if trimmed == "" {
		return display
	}
	return trimmed
}
