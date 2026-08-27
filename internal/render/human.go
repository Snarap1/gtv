package render

import (
	"fmt"
	"io"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
)

type HumanOptions struct {
	Color bool
	Options
}

func DefaultHumanOptions() HumanOptions {
	return HumanOptions{Color: true, Options: DefaultOptions()}
}

func Human(w io.Writer, t *model.Tree, opts HumanOptions) {
	c := t.Counts()
	status, col := "PASS", ansiGreen
	if c.Failed > 0 {
		status, col = "FAIL", ansiRed
	}
	fmt.Fprintf(w, "%s  %s\n\n", colorize(opts.Color, col, status), summary(c, t.Duration()))

	suites := t.Suites()
	for i, s := range suites {
		renderNode(w, s, "", i == len(suites)-1, opts)
	}

	failed, _ := partition(t.Leaves())
	for _, n := range failed {
		fmt.Fprintln(w)
		writeFailure(w, n, opts.Options)
	}
}

func renderNode(w io.Writer, n *model.Node, prefix string, last bool, opts HumanOptions) {
	branch, cont := "├─ ", "│  "
	if last {
		branch, cont = "└─ ", "   "
	}

	if g, ok := paramGroup(n); ok {
		writeGroupLine(w, prefix, branch, n, g, opts)
		if g.failed > 0 {
			renderFailedInvocations(w, n, prefix+cont, g, opts)
		}
		return
	}

	if n.IsTest {
		writeLeafLine(w, prefix, branch, n, opts)
		return
	}

	writeSuiteLine(w, prefix, branch, n, opts)
	for i, c := range n.Children {
		renderNode(w, c, prefix+cont, i == len(n.Children)-1, opts)
	}
}

func renderFailedInvocations(w io.Writer, n *model.Node, prefix string, g groupCounts, opts HumanOptions) {
	var failed []*model.Node
	for _, c := range n.Children {
		if c.Res == event.Failure {
			failed = append(failed, c)
		}
	}
	hidden := g.total - g.failed
	for i, c := range failed {
		renderNode(w, c, prefix, i == len(failed)-1 && hidden == 0, opts)
	}
	if hidden > 0 {
		fmt.Fprintf(w, "%s└─ %s\n", prefix, dimText(opts.Color, fmt.Sprintf("+%d ok", hidden)))
	}
}

type groupCounts struct{ total, ok, failed, skipped int }

func paramGroup(n *model.Node) (groupCounts, bool) {
	if n.IsTest || n.Scaffolding() || len(n.Children) == 0 {
		return groupCounts{}, false
	}
	if n.Parent == nil || n.Cls == "" || n.Cls != n.Parent.Cls {
		return groupCounts{}, false
	}
	var g groupCounts
	for _, c := range n.Children {
		if !c.IsTest {
			return groupCounts{}, false
		}
		g.total++
		switch c.Res {
		case event.Success:
			g.ok++
		case event.Failure:
			g.failed++
		case event.Skipped:
			g.skipped++
		}
	}
	return g, true
}

func writeSuiteLine(w io.Writer, prefix, branch string, n *model.Node, opts HumanOptions) {
	label := colorize(opts.Color, suiteColor(n), n.Label())
	fmt.Fprintf(w, "%s%s%s  %s\n", prefix, branch, label, dimText(opts.Color, suiteMeta(n)))
}

func suiteColor(n *model.Node) string {
	switch {
	case n.Failed > 0:
		return ansiRed
	case n.Total == 0:
		return ansiDim
	case n.Ok == 0 && n.Skipped > 0:
		return ansiYellow
	default:
		return ansiGreen
	}
}

func suiteMeta(n *model.Node) string {
	if n.Total == 0 {
		return ""
	}
	counts := fmt.Sprintf("%d/%d ok", n.Ok, n.Total)
	if n.Failed > 0 {
		counts += fmt.Sprintf(", %d failed", n.Failed)
	}
	if n.Skipped > 0 {
		counts += fmt.Sprintf(", %d skip", n.Skipped)
	}
	return fmt.Sprintf("(%s, %s)", counts, Duration(n.Duration()))
}

func writeLeafLine(w io.Writer, prefix, branch string, n *model.Node, opts HumanOptions) {
	sym, col := statusSymbol(n.Res)
	line := fmt.Sprintf("%s %s", colorize(opts.Color, col, sym), n.Label())
	fmt.Fprintf(w, "%s%s%s  %s\n", prefix, branch, line, dimText(opts.Color, Duration(n.Duration())))
}

func writeGroupLine(w io.Writer, prefix, branch string, n *model.Node, g groupCounts, opts HumanOptions) {
	sym, col, label := "✓", ansiGreen, fmt.Sprintf("%d/%d ok", g.ok, g.total)
	switch {
	case g.failed > 0:
		sym, col, label = "✗", ansiRed, fmt.Sprintf("%d/%d failed", g.failed, g.total)
	case g.ok == 0 && g.skipped > 0:
		sym, col = "○", ansiYellow
	}
	line := fmt.Sprintf("%s %s  %s", colorize(opts.Color, col, sym), n.Label(), label)
	fmt.Fprintf(w, "%s%s%s  %s\n", prefix, branch, line, dimText(opts.Color, Duration(n.Duration())))
}

func statusSymbol(res string) (string, string) {
	switch res {
	case event.Success:
		return "✓", ansiGreen
	case event.Failure:
		return "✗", ansiRed
	case event.Skipped:
		return "○", ansiYellow
	default:
		return "?", ansiDim
	}
}

func colorize(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + ansiReset
}

func dimText(on bool, s string) string {
	if s == "" {
		return ""
	}
	return colorize(on, ansiDim, s)
}
