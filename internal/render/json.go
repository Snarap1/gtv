package render

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/pavelnaibich/gtv/internal/model"
)

type jsonReport struct {
	Status     string     `json:"status"`
	Tasks      []string   `json:"tasks"`
	Total      int        `json:"total"`
	Ok         int        `json:"ok"`
	Failed     int        `json:"failed"`
	Skipped    int        `json:"skipped"`
	DurationMs int64      `json:"duration_ms"`
	Suites     []jsonNode `json:"suites"`
}

type jsonNode struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Status     string     `json:"status,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	Total      int        `json:"total,omitempty"`
	Ok         int        `json:"ok,omitempty"`
	Failed     int        `json:"failed,omitempty"`
	Skipped    int        `json:"skipped,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Failures   []jsonFail `json:"failures,omitempty"`
	Output     []string   `json:"output,omitempty"`
	Children   []jsonNode `json:"children,omitempty"`
}

type jsonFail struct {
	Message  string   `json:"message"`
	Class    string   `json:"class,omitempty"`
	Expected string   `json:"expected,omitempty"`
	Actual   string   `json:"actual,omitempty"`
	Causes   []string `json:"causes,omitempty"`
	Stack    string   `json:"stack,omitempty"`
}

func JSON(w io.Writer, t *model.Tree, opts Options) error {
	c := t.Counts()
	status := "PASS"
	if c.Failed > 0 {
		status = "FAIL"
	}
	rep := jsonReport{
		Status: status, Tasks: t.Tasks(),
		Total: c.Total, Ok: c.Ok, Failed: c.Failed, Skipped: c.Skipped,
		DurationMs: t.Duration(),
	}
	for _, s := range t.Suites() {
		rep.Suites = append(rep.Suites, toJSONNode(s, opts))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func toJSONNode(n *model.Node, opts Options) jsonNode {
	jn := jsonNode{Name: n.Label(), DurationMs: n.Duration()}

	if n.IsTest {
		jn.Type = "test"
		jn.Status = n.Res
		for _, f := range n.Failures {
			jn.Failures = append(jn.Failures, jsonFail{
				Message:  strings.TrimSpace(f.Msg),
				Class:    f.Cls,
				Expected: f.Expected,
				Actual:   f.Actual,

				Causes: Causes(f.Stack, 0),
				Stack:  strings.Join(Frames(f.Stack, opts.MaxFrames), "\n"),
			})
		}
		if n.Assumed != nil {
			jn.Reason = strings.TrimSpace(n.Assumed.Msg)
		}
		if opts.ShowOutput {
			jn.Output = TailLines(n.Out, opts.OutputLines)
		}
		return jn
	}

	jn.Type = "suite"
	jn.Total, jn.Ok, jn.Failed, jn.Skipped = n.Total, n.Ok, n.Failed, n.Skipped
	for _, c := range n.Children {
		jn.Children = append(jn.Children, toJSONNode(c, opts))
	}
	return jn
}
