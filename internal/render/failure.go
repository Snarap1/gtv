package render

import (
	"regexp"
	"strings"

	"github.com/pavelnaibich/gtv/internal/event"
)

// ShortClass drops the package from an exception class name.
func ShortClass(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// Headline is the one-or-few-line description of a failure: "AssertionError:
// Expected size: 5 but was: 3". AssertJ messages start with a newline and carry
// the useful detail across several lines, so keep up to maxLines of them.
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

// Kotlin method names can contain spaces, so the frame regex cannot assume an
// identifier before the parenthesis.
var frameRe = regexp.MustCompile(`^\s*at\s+(.+)\(([^()]*)\)\s*$`)

var noisyPrefix = []string{
	"java.", "javax.", "jdk.", "sun.", "kotlin.", "kotlinx.coroutines.",
	"org.junit", "junit.", "org.gradle", "worker.org.gradle", "org.mockito", "io.mockk.impl",
}

// Frames extracts the source locations worth showing, closest to the failure
// first. Gradle already strips most framework noise; this drops what is left.
func Frames(stack string, max int) []string {
	var out []string
	for _, line := range strings.Split(stack, "\n") {
		m := frameRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		owner, loc := m[1], m[2]
		if !strings.Contains(loc, ":") {
			continue // "Native Method", "Unknown Source"
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

// TailLines returns the last n non-empty lines of captured test output.
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
