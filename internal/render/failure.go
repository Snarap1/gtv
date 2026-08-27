package render

import (
	"regexp"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
)

func ShortClass(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

func Headline(f event.Fail, maxLines int) []string {
	msg := strings.TrimSpace(f.Msg)
	cls := ShortClass(f.Cls)

	var lines []string
	for _, l := range strings.Split(msg, "\n") {
		if l = strings.TrimRight(l, " \t"); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return []string{cls}
	}
	if cls != "" && !strings.HasPrefix(lines[0], cls) {
		lines[0] = cls + ": " + lines[0]
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines:maxLines], "…")
	}
	return lines
}

var causeRe = regexp.MustCompile(`^\s*Caused by:\s+([^\s:]+)(?::\s*(.*))?$`)

func Causes(stack string, max int) []string {
	var chain []string
	for _, line := range strings.Split(stack, "\n") {
		m := causeRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		text := ShortClass(m[1])
		if msg := strings.TrimSpace(m[2]); msg != "" {
			text += ": " + msg
		}
		chain = append(chain, text)
	}
	if len(chain) == 0 {
		return nil
	}

	var kept []string
	for i := len(chain) - 1; i >= 0; i-- {
		if len(kept) > 0 && strings.Contains(chain[i], kept[len(kept)-1]) {
			continue
		}
		kept = append(kept, chain[i])
		if max > 0 && len(kept) == max {
			break
		}
	}

	for l, r := 0, len(kept)-1; l < r; l, r = l+1, r-1 {
		kept[l], kept[r] = kept[r], kept[l]
	}
	return kept
}

var frameRe = regexp.MustCompile(`^\s*at\s+(.+)\(([^()]*)\)\s*$`)

var noisyPrefix = []string{
	"java.", "javax.", "jdk.", "sun.", "kotlin.", "kotlinx.coroutines.",
	"org.junit", "junit.", "org.gradle", "worker.org.gradle", "org.mockito", "io.mockk.impl",
}

func Frames(stack string, max int) []string {
	var out []string
	for _, line := range strings.Split(stack, "\n") {
		m := frameRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		owner, loc := m[1], m[2]
		if !strings.Contains(loc, ":") {
			continue
		}
		if hasAnyPrefix(owner, noisyPrefix) {
			continue
		}
		out = append(out, loc)
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func TailLines(chunks []string, n int) []string {
	var lines []string
	for _, c := range chunks {
		for _, l := range strings.Split(c, "\n") {
			if l = strings.TrimRight(l, " \t\r"); l != "" {
				lines = append(lines, l)
			}
		}
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
