package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

type Options struct {
	MaxFailures int
	MaxSkipped  int
	MaxMsgLines int
	MaxCauses   int
	MaxFrames   int
	ShowOutput  bool
	OutputLines int
}

func DefaultOptions() Options {
	return Options{MaxFailures: 10, MaxSkipped: 5, MaxMsgLines: 6, MaxCauses: 2, MaxFrames: 3, OutputLines: 10}
}

func Agent(w io.Writer, t *model.Tree, opts Options) {
	c := t.Counts()
	status := "PASS"
	if c.Failed > 0 {
		status = "FAIL"
	}
	fmt.Fprintf(w, "%s %s  %s\n", status, strings.Join(t.Tasks(), " "), summary(c, t.Duration()))

	failed, skipped := partition(t.Leaves())

	shown := failed
	if opts.MaxFailures > 0 && len(shown) > opts.MaxFailures {
		shown = shown[:opts.MaxFailures]
	}
	for _, n := range shown {
		writeFailure(w, n, opts)
	}
	if rest := len(failed) - len(shown); rest > 0 {
		fmt.Fprintf(w, "  +%d more failures\n", rest)
	}

	if len(skipped) > 0 {
		writeSkipped(w, skipped, opts)
	}
}

func summary(c model.Counts, durMs int64) string {
	parts := []string{fmt.Sprintf("%d/%d ok", c.Ok, c.Total)}
	if c.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d fail", c.Failed))
	}
	if c.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skip", c.Skipped))
	}
	return fmt.Sprintf("%s (%s)", strings.Join(parts, ", "), Duration(durMs))
}

func Duration(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm%02ds", ms/60_000, (ms%60_000)/1000)
	}
}

func partition(leaves []*model.Node) (failed, skipped []*model.Node) {
	for _, n := range leaves {
		switch n.Res {
		case event.Failure:
			failed = append(failed, n)
		case event.Skipped:
			skipped = append(skipped, n)
		}
	}
	return failed, skipped
}

func writeFailure(w io.Writer, n *model.Node, opts Options) {
	fmt.Fprintf(w, "✗ %s\n", strings.Join(n.Path(), " > "))
	for _, f := range n.Failures {
		for _, line := range Headline(f, opts.MaxMsgLines) {
			fmt.Fprintf(w, "  %s\n", line)
		}
		if wantsDiff(f) {
			fmt.Fprintf(w, "  expected: %s\n  actual:   %s\n", f.Expected, f.Actual)
		}
		for _, c := range Causes(f.Stack, opts.MaxCauses) {
			fmt.Fprintf(w, "  caused by: %s\n", c)
		}
		if frames := Frames(f.Stack, opts.MaxFrames); len(frames) > 0 {
			fmt.Fprintf(w, "  %s\n", strings.Join(frames, " <- "))
		}
	}
	if opts.ShowOutput {
		for _, line := range TailLines(n.Out, opts.OutputLines) {
			fmt.Fprintf(w, "  | %s\n", line)
		}
	}
}

func wantsDiff(f event.Fail) bool {
	if f.Expected == "" && f.Actual == "" {
		return false
	}
	return !(strings.Contains(f.Msg, f.Expected) && strings.Contains(f.Msg, f.Actual))
}

func writeSkipped(w io.Writer, skipped []*model.Node, opts Options) {
	shown := skipped
	if opts.MaxSkipped > 0 && len(shown) > opts.MaxSkipped {
		shown = shown[:opts.MaxSkipped]
	}
	for _, n := range shown {
		line := "~ " + strings.Join(n.Path(), " > ")

		if n.Assumed != nil {
			if msg := strings.TrimSpace(n.Assumed.Msg); msg != "" {
				line += " — " + firstLine(msg)
			}
		}
		fmt.Fprintln(w, line)
	}
	if rest := len(skipped) - len(shown); rest > 0 {
		fmt.Fprintf(w, "  +%d more skipped\n", rest)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
