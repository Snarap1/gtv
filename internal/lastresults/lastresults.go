// Package lastresults reads the JUnit XML reports Gradle already wrote under
// build/test-results, instead of running the tests again. It folds them into
// the same model.Tree a live run builds, so both renderers work unchanged.
package lastresults

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

// ErrNoResults means the test-results directory has no XML reports (or none
// matching the target) — the tests in question have never been run.
var ErrNoResults = errors.New("no test results found; run without --last first")

// Dir maps a Gradle task path to the directory Gradle writes its JUnit XML
// reports to, e.g. ":a:b:test" -> "<root>/a/b/build/test-results/test".
func Dir(root, task string) string {
	task = strings.TrimPrefix(task, ":")
	parts := strings.Split(task, ":")
	name := parts[len(parts)-1]
	segs := append(append([]string{root}, parts[:len(parts)-1]...), "build", "test-results", name)
	return filepath.Join(segs...)
}

// Load reads every TEST-*.xml report in dir whose class matches filter — a
// bare class FQN, "FQN.method", or "" for every report in dir — and folds
// them into a tree shaped the same way a live run would build it.
func Load(dir, task, filter string) (*model.Tree, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNoResults
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "TEST-") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)

	t := model.New()
	var offset int64
	matched := 0
	for _, f := range files {
		ts, err := parse(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(f), err)
		}
		if filter != "" && ts.Name != filter && !strings.HasPrefix(filter, ts.Name+".") {
			continue
		}
		matched++
		offset, _, _, _, _ = emit(t, task, f, buildTrie(ts.Name, ts.Cases), "", offset)
	}
	if matched == 0 {
		return nil, ErrNoResults
	}
	return t, nil
}

type testsuiteXML struct {
	XMLName xml.Name      `xml:"testsuite"`
	Name    string        `xml:"name,attr"`
	Cases   []testcaseXML `xml:"testcase"`
}

type testcaseXML struct {
	Name      string      `xml:"name,attr"`
	Classname string      `xml:"classname,attr"`
	Time      string      `xml:"time,attr"`
	Failure   *failureXML `xml:"failure"`
	Error     *failureXML `xml:"error"`
	Skipped   *skippedXML `xml:"skipped"`
}

type failureXML struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type skippedXML struct {
	Message string `xml:"message,attr"`
}

func parse(path string) (testsuiteXML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return testsuiteXML{}, err
	}
	var ts testsuiteXML
	if err := xml.Unmarshal(data, &ts); err != nil {
		return testsuiteXML{}, err
	}
	return ts, nil
}

// classNode is one level of the class hierarchy reconstructed from
// classname attributes: the XML report has no structural nesting, only
// "Outer" and "Outer$Nested1$Nested2" strings to split apart.
type classNode struct {
	cls      string
	label    string
	children map[string]*classNode
	order    []string
	tests    []testcaseXML
}

// buildTrie groups a report's testcases by class, using the outer class's own
// name as the root — including it even when it directly holds no test cases
// of its own, since @Nested children still need it as their parent.
func buildTrie(outerFQN string, cases []testcaseXML) *classNode {
	label := outerFQN
	if i := strings.LastIndex(label, "."); i >= 0 {
		label = label[i+1:]
	}
	root := &classNode{cls: outerFQN, label: label, children: map[string]*classNode{}}
	for _, tc := range cases {
		cls := tc.Classname
		if cls == "" {
			cls = outerFQN
		}
		cur := root
		built := outerFQN
		for _, seg := range strings.Split(strings.TrimPrefix(strings.TrimPrefix(cls, outerFQN), "$"), "$") {
			if seg == "" {
				continue
			}
			built += "$" + seg
			child, ok := cur.children[seg]
			if !ok {
				child = &classNode{cls: built, label: seg, children: map[string]*classNode{}}
				cur.children[seg] = child
				cur.order = append(cur.order, seg)
			}
			cur = child
		}
		cur.tests = append(cur.tests, tc)
	}
	return root
}

// emit walks the trie depth-first, feeding synthetic events into t exactly
// like a live run would — a SuiteStart/SuiteEnd pair per class level and a
// TestStart/TestEnd pair per test case — so every field a renderer reads
// (Total/Ok/Failed/Skipped, Start/End) comes from the same code path as a
// real run instead of being poked in after the fact.
//
// The XML has no wall-clock timestamps, only each test's own duration, so
// offset is a running clock: every leaf advances it by its own time and every
// suite's Start/End is simply where that clock stood when it began and ended.
// That makes the tree's total duration the sum of every test's reported time
// rather than true wall time, which is unknowable from the report alone.
func emit(t *model.Tree, task, file string, n *classNode, parent string, offset int64) (newOffset, total, ok, failed, skipped int64) {
	key := file + "::" + n.cls
	t.Apply(event.Event{E: event.SuiteStart, Task: task, Key: key, Parent: parent, Name: n.label, Display: n.label, Cls: n.cls})

	start := offset
	for _, seg := range n.order {
		var ct, co, cf, cs int64
		offset, ct, co, cf, cs = emit(t, task, file, n.children[seg], key, offset)
		total += ct
		ok += co
		failed += cf
		skipped += cs
	}
	for i, tc := range n.tests {
		leafKey := fmt.Sprintf("%s::%d", key, i)
		res, fails, assumed := classify(tc)
		dur := parseMillis(tc.Time)
		leafStart := offset
		offset += dur
		t.Apply(event.Event{E: event.TestStart, Task: task, Key: leafKey, Parent: key, Name: tc.Name, Display: tc.Name, Cls: n.cls})
		t.Apply(event.Event{E: event.TestEnd, Task: task, Key: leafKey, Res: res, Start: leafStart, End: offset, Failures: fails, Assumed: assumed})
		total++
		switch res {
		case event.Success:
			ok++
		case event.Failure:
			failed++
		case event.Skipped:
			skipped++
		}
	}

	t.Apply(event.Event{E: event.SuiteEnd, Task: task, Key: key, Start: start, End: offset, Total: total, Ok: ok, Failed: failed, SkipCnt: skipped})
	return offset, total, ok, failed, skipped
}

func classify(tc testcaseXML) (res string, fails []event.Fail, assumed *event.Fail) {
	switch {
	case tc.Failure != nil:
		return event.Failure, []event.Fail{{
			Msg:       firstNonEmpty(tc.Failure.Message, tc.Failure.Text),
			Cls:       tc.Failure.Type,
			Stack:     tc.Failure.Text,
			Assertion: strings.Contains(strings.ToLower(tc.Failure.Type), "assert"),
		}}, nil
	case tc.Error != nil:
		return event.Failure, []event.Fail{{
			Msg:   firstNonEmpty(tc.Error.Message, tc.Error.Text),
			Cls:   tc.Error.Type,
			Stack: tc.Error.Text,
		}}, nil
	case tc.Skipped != nil:
		if tc.Skipped.Message == "" {
			return event.Skipped, nil, nil
		}
		return event.Skipped, nil, &event.Fail{Msg: tc.Skipped.Message, Assumption: true}
	default:
		return event.Success, nil, nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseMillis(s string) int64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1000)
}
