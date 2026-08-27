package model

import (
	"regexp"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
)

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

	Total, Ok, Failed, Skipped int
}

func (n *Node) Scaffolding() bool { return !n.IsTest && n.Cls == "" }

func (n *Node) Label() string { return trimSignature(n.Display) }

func (n *Node) Duration() int64 {
	if n.End > n.Start {
		return n.End - n.Start
	}
	return 0
}

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

const (
	maxOutChunks    = 200
	maxOutChunkLen  = 4096
	maxTreeOutBytes = 8 * 1024 * 1024
)

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

type Tree struct {
	byKey    map[string]*Node
	roots    []*Node
	tasks    []string
	outBytes int
}

func New() *Tree { return &Tree{byKey: map[string]*Node{}} }

func (t *Tree) Roots() []*Node { return t.roots }

func (t *Tree) Tasks() []string { return t.tasks }

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

type Counts struct{ Total, Ok, Failed, Skipped int }

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
