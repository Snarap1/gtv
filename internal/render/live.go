package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/model"
)

// Live redraws a bounded tail of finished tests as events arrive, using
// carriage-return/erase-line ANSI codes rather than an alt-screen — the
// terminal's own scrollback stays usable. It keeps its own tree, built from
// the same events the runner folds into its result tree, since OnEvent only
// hands over one event at a time.
type Live struct {
	w        io.Writer
	color    bool
	maxLines int
	tree     *model.Tree
	drawn    int
}

// NewLive returns a Live renderer. maxLines bounds how many finished tests
// stay visible above the running summary line.
func NewLive(w io.Writer, color bool, maxLines int) *Live {
	return &Live{w: w, color: color, maxLines: maxLines, tree: model.New()}
}

// Handle folds one event into the live tree and redraws.
func (l *Live) Handle(e event.Event) {
	l.tree.Apply(e)
	l.redraw()
}

func (l *Live) redraw() {
	lines := l.frame()
	if l.drawn > 0 {
		fmt.Fprintf(l.w, "\x1b[%dA", l.drawn)
	}
	for _, ln := range lines {
		fmt.Fprintf(l.w, "\r\x1b[K%s\n", ln)
	}
	l.drawn = len(lines)
}

func (l *Live) frame() []string {
	var finished []*model.Node
	for _, n := range l.tree.Leaves() {
		if n.Res != "" {
			finished = append(finished, n)
		}
	}
	if len(finished) > l.maxLines {
		finished = finished[len(finished)-l.maxLines:]
	}

	lines := make([]string, 0, len(finished)+1)
	for _, n := range finished {
		sym, col := statusSymbol(n.Res)
		lines = append(lines, fmt.Sprintf("%s %s", colorize(l.color, col, sym), strings.Join(n.Path(), " > ")))
	}
	lines = append(lines, dimText(l.color, liveSummary(l.tree.Counts())))
	return lines
}

func liveSummary(c model.Counts) string {
	parts := []string{fmt.Sprintf("%d/%d ok", c.Ok, c.Total)}
	if c.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d fail", c.Failed))
	}
	if c.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skip", c.Skipped))
	}
	return strings.Join(parts, ", ")
}

// Finish erases the live progress area so the final static render can take
// its place cleanly.
func (l *Live) Finish() {
	if l.drawn == 0 {
		return
	}
	fmt.Fprintf(l.w, "\x1b[%dA", l.drawn)
	for i := 0; i < l.drawn; i++ {
		fmt.Fprint(l.w, "\x1b[K\n")
	}
	fmt.Fprintf(l.w, "\x1b[%dA", l.drawn)
	l.drawn = 0
}
