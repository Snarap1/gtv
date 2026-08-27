// Command gtv runs Gradle tests and reports them in a form built for reading —
// by a person or by a coding agent — instead of Gradle's default silence.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pavelnaibich/gtv/internal/lastresults"
	"github.com/pavelnaibich/gtv/internal/model"
	"github.com/pavelnaibich/gtv/internal/render"
	"github.com/pavelnaibich/gtv/internal/runner"
	"github.com/pavelnaibich/gtv/internal/stats"
	"github.com/pavelnaibich/gtv/internal/target"
	"github.com/pavelnaibich/gtv/internal/watch"
)

const usage = `gtv — Gradle test runner with readable output

usage: gtv [flags] <target> [gradle args...]
       gtv --stats
       gtv stats

<target> is a Gradle task path, a class name/FQN, a path to a test file, or
"Class.method" / "Class::method".

examples:
  gtv UserServiceTest
  gtv UserServiceTest.should pass
  gtv :app:service:test --tests "*.UserServiceTest"
  gtv :lib:test
  gtv --stats

flags:`

// version is set at build time via -ldflags "-X main.version=...";
// see build.sh, which fills it in from the nearest git tag.
var version = "dev"

func main() {
	opts := render.DefaultOptions()
	var (
		javaMajor    = flag.Int("java", 21, "minimum JDK major version to build with")
		noRerun      = flag.Bool("no-rerun", false, "let Gradle skip UP-TO-DATE or cached test tasks")
		gradleOutput = flag.Bool("gradle-output", false, "always print Gradle's own output")
		forceAgent   = flag.Bool("agent", false, "force compact agent-oriented output")
		forceHuman   = flag.Bool("human", false, "force colored tree output")
		reindex      = flag.Bool("reindex", false, "rebuild the test class index instead of trusting the cache")
		last         = flag.Bool("last", false, "read the previous run's JUnit XML reports instead of running Gradle")
		jsonOut      = flag.Bool("json", false, "print the machine-readable tree instead of human/agent text")
		watchFlag    = flag.Bool("watch", false, "rerun whenever files under the project change")
		showVersion  = flag.Bool("version", false, "print the gtv version and exit")
		showStats    = flag.Bool("stats", false, "print cumulative token-savings stats and exit")
	)
	flag.BoolVar(&opts.ShowOutput, "test-output", false, "print captured stdout/stderr of failed tests")
	flag.IntVar(&opts.MaxFailures, "max-fail", opts.MaxFailures, "failures to report in full (0 = all)")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("gtv", version)
		return
	}
	if *showStats || (flag.NArg() == 1 && flag.Arg(0) == "stats") {
		os.Exit(printStats())
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *forceAgent && *forceHuman {
		fmt.Fprintln(os.Stderr, "gtv: --agent and --human are mutually exclusive")
		os.Exit(2)
	}

	tty := isTTY(os.Stdout)
	human := wantHuman(*forceAgent, *forceHuman, tty)
	color := human && wantColor(tty)

	runOnce := func() int {
		if *last {
			return runLast(flag.Args(), human, color, *reindex, *jsonOut, opts)
		}
		return run(flag.Args(), *javaMajor, *noRerun, *gradleOutput, human, color, *reindex, *jsonOut, opts)
	}

	if !*watchFlag {
		os.Exit(runOnce())
	}

	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(fatal(err))
	}
	root, err := runner.FindGradleRoot(cwd)
	if err != nil {
		os.Exit(fatal(err))
	}
	first := true
	watch.Until(root, func() {
		if !first {
			fmt.Printf("\n── rerun %s ──\n\n", time.Now().Format("15:04:05"))
		}
		first = false
		runOnce()
	})
}

// wantHuman picks the tree renderer for an interactive terminal and the
// compact one otherwise; CI and coding-agent harnesses look like a tty but
// want the compact form, so their env vars override the tty check.
func wantHuman(forceAgent, forceHuman, tty bool) bool {
	switch {
	case forceHuman:
		return true
	case forceAgent:
		return false
	case os.Getenv("CI") != "", os.Getenv("CLAUDE_CODE") != "":
		return false
	default:
		return tty
	}
}

func wantColor(tty bool) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return tty
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func run(args []string, javaMajor int, noRerun, alwaysShowGradle, human, color, reindex, jsonOut bool, opts render.Options) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fatal(err)
	}
	root, err := runner.FindGradleRoot(cwd)
	if err != nil {
		return fatal(err)
	}
	jdk, err := runner.FindJavaHome(javaMajor)
	if err != nil {
		return fatal(err)
	}

	gradleArgs, err := resolveArgs(root, args, reindex)
	if err != nil {
		return fatal(err)
	}

	cfg := runner.Config{Root: root, JavaHome: jdk.Home, Args: gradleArgs, ForceRerun: !noRerun, CaptureOutput: opts.ShowOutput}

	// Live progress only makes sense when the final frame lands on the same
	// terminal: a coding agent or a redirected file gets the static render only.
	var live *render.Live
	if human && isTTY(os.Stdout) {
		live = render.NewLive(os.Stdout, color, 12)
		cfg.OnEvent = live.Handle
	}

	// Forcing the rerun happens inside the init script rather than via Gradle's
	// --rerun flag, which would only cover the task named on the command line.
	res, err := runner.Execute(cfg)
	if live != nil {
		live.Finish()
	}
	if err != nil {
		return fatal(err)
	}

	// Gradle still emits its scaffolding suites when a filter matches nothing, so
	// an empty run has to be judged by the test count, not by the event count.
	if res.Tree.Counts().Total == 0 {
		fmt.Printf("NOTESTS %s\n", strings.Join(args, " "))
		fmt.Print(indent(reason(res.GradleOutput)))
		return 1
	}

	if err := writeReport(res.Tree, jsonOut, human, color, opts); err != nil {
		return fatal(err)
	}
	recordSavings(root, res.GradleBytes, res.Tree, opts)
	if alwaysShowGradle {
		fmt.Print(indent(res.GradleOutput))
	}

	if res.Tree.Counts().Failed > 0 {
		return 1
	}
	if res.ExitCode != 0 {
		// Keep --json stdout parseable even when a non-test Gradle task fails.
		fmt.Fprint(os.Stderr, indent(reason(res.GradleOutput)))
	}
	return res.ExitCode
}

// recordSavings compares the agent report to the full Gradle console from the
// same run and persists the counters. Baseline is real captured bytes (not an
// NDJSON estimate and not a second Gradle launch).
func recordSavings(root string, gradleBytes int64, tree *model.Tree, opts render.Options) {
	actual := agentReportBytes(tree, opts)
	if err := stats.Record(root, gradleBytes, actual); err != nil {
		fmt.Fprintf(os.Stderr, "gtv: stats: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, stats.RunLine(gradleBytes, actual))
}

func agentReportBytes(tree *model.Tree, opts render.Options) int64 {
	var buf bytes.Buffer
	render.Agent(&buf, tree, opts)
	return int64(buf.Len())
}

func printStats() int {
	f, err := stats.Load()
	if err != nil {
		return fatal(err)
	}
	fmt.Print(stats.Format(f, stats.Session()))
	return 0
}

// resolveArgs turns the leading target argument into a Gradle task path
// (plus a --tests filter, when the target names a class rather than a task)
// and appends any remaining gradle args the caller passed after it.
func resolveArgs(root string, args []string, reindex bool) ([]string, error) {
	t, err := resolveTarget(root, args[0], reindex)
	if err != nil {
		return nil, err
	}
	out := []string{t.Task}
	if t.TestFilter != "" {
		out = append(out, "--tests", t.TestFilter)
	}
	return append(out, args[1:]...), nil
}

// resolveTarget wraps target.Resolve, folding an ambiguous match's candidate
// list into the error message so every caller reports it the same way.
func resolveTarget(root, arg string, reindex bool) (target.Target, error) {
	t, cands, err := target.Resolve(root, arg, reindex)
	if err == nil {
		return t, nil
	}
	if errors.Is(err, target.ErrAmbiguous) {
		var b strings.Builder
		fmt.Fprintf(&b, "%v:\n", err)
		for _, c := range cands {
			fmt.Fprintf(&b, "  %s (%s)\n", c.FQN, c.File)
		}
		return target.Target{}, errors.New(strings.TrimRight(b.String(), "\n"))
	}
	return target.Target{}, err
}

// writeReport picks the renderer: JSON wins outright, since it is a distinct
// machine-readable mode rather than a variant of human/agent text.
func writeReport(t *model.Tree, jsonOut, human, color bool, opts render.Options) error {
	switch {
	case jsonOut:
		return render.JSON(os.Stdout, t, opts)
	case human:
		render.Human(os.Stdout, t, render.HumanOptions{Color: color, Options: opts})
	default:
		render.Agent(os.Stdout, t, opts)
	}
	return nil
}

// runLast reads the previous run's JUnit XML reports instead of invoking
// Gradle, for --last: no build, no wait, just what the last real run wrote.
func runLast(args []string, human, color, reindex, jsonOut bool, opts render.Options) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fatal(err)
	}
	root, err := runner.FindGradleRoot(cwd)
	if err != nil {
		return fatal(err)
	}
	t, err := resolveTarget(root, args[0], reindex)
	if err != nil {
		return fatal(err)
	}

	dir := lastresults.Dir(root, t.Task)
	tree, err := lastresults.Load(dir, t.Task, t.TestFilter)
	if err != nil {
		if errors.Is(err, lastresults.ErrNoResults) {
			fmt.Fprintf(os.Stderr, "gtv: %v: %s\n", err, dir)
			return 1
		}
		return fatal(err)
	}

	if tree.Counts().Total == 0 {
		fmt.Printf("NOTESTS %s\n", strings.Join(args, " "))
		return 1
	}
	if err := writeReport(tree, jsonOut, human, color, opts); err != nil {
		return fatal(err)
	}
	if tree.Counts().Failed > 0 {
		return 1
	}
	return 0
}

func fatal(err error) int {
	fmt.Fprintln(os.Stderr, "gtv:", err)
	return 2
}

// reason pulls Gradle's "What went wrong" section — the one part of a failed
// build worth reading — falling back to the tail of the log when the build died
// without one.
func reason(output string) string {
	// A compile failure reports only "See log for more details" in its section,
	// while the diagnostics themselves sit further up the log.
	if errs := compileErrors(output); len(errs) > 0 {
		return strings.Join(errs, "\n")
	}

	const header = "* What went wrong:"
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, header) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return tail(output, 25)
	}
	var section []string
	for _, line := range lines[start:] {
		if strings.HasPrefix(line, "* ") {
			break
		}
		if strings.TrimSpace(line) != "" {
			section = append(section, strings.TrimPrefix(line, "> "))
		}
	}
	return strings.Join(section, "\n")
}

// Kotlin reports "e: file:///abs/path/File.kt:56:31 message"; javac reports
// "/abs/path/File.java:56: error: message".
var (
	kotlinError = regexp.MustCompile(`^e: (?:file://)?(\S+?):(\d+(?::\d+)?):?\s*(.*)$`)
	javacError  = regexp.MustCompile(`^(\S+\.java):(\d+):\s*error:\s*(.*)$`)
)

const maxCompileErrors = 10

// compileErrors extracts compiler diagnostics, shortening absolute paths to the
// file name so a line stays readable.
func compileErrors(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		m := kotlinError.FindStringSubmatch(line)
		if m == nil {
			m = javacError.FindStringSubmatch(line)
		}
		if m == nil {
			continue
		}
		file := m[1]
		if i := strings.LastIndexAny(file, `/\`); i >= 0 {
			file = file[i+1:]
		}
		out = append(out, fmt.Sprintf("%s:%s %s", file, m[2], m[3]))
		if len(out) == maxCompileErrors {
			out = append(out, "…")
			break
		}
	}
	return out
}

// tail keeps the last n lines, which is where Gradle puts the reason a build died.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func indent(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}
