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
       gtv compile <module> [gradle args...]
       gtv build [gradle args...]
       gtv --stats
       gtv stats

<target> is a Gradle task path, a class name/FQN, a path to a test file, or
"Class.method" / "Class::method".

"compile <module>" builds one module without running its tests
(<module>:build -x test); <module> is a Gradle module path, class name/FQN,
or source file, same resolution as <target> minus the method part.

"build" runs the whole project's "build" task from the repo root.

examples:
  gtv UserServiceTest
  gtv UserServiceTest.should pass
  gtv :app:service:test --tests "*.UserServiceTest"
  gtv :lib:test
  gtv compile UserServiceTest
  gtv compile :app:service
  gtv build
  gtv --stats

flags:`

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

	if flag.Arg(0) == "compile" {
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "gtv: compile requires a module argument, e.g. gtv compile :app:service")
			os.Exit(2)
		}
		os.Exit(runCompile(flag.Args()[1:], *javaMajor, *reindex, *gradleOutput, human, color, opts))
	}
	if flag.Arg(0) == "build" {
		os.Exit(runBuild(flag.Args()[1:], *javaMajor, *gradleOutput, human, color, opts))
	}

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

	var live *render.Live
	if human && isTTY(os.Stdout) {
		live = render.NewLive(os.Stdout, color, 12)
		cfg.OnEvent = live.Handle
	}

	res, err := runner.Execute(cfg)
	if live != nil {
		live.Finish()
	}
	if err != nil {
		return fatal(err)
	}

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

		fmt.Fprint(os.Stderr, indent(reason(res.GradleOutput)))
	}
	return res.ExitCode
}

func runCompile(args []string, javaMajor int, reindex, alwaysShowGradle, human, color bool, opts render.Options) int {
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

	module, cands, err := target.ResolveModule(root, args[0], reindex)
	if err != nil {
		if errors.Is(err, target.ErrAmbiguous) {
			var b strings.Builder
			fmt.Fprintf(&b, "%v:\n", err)
			for _, c := range cands {
				fmt.Fprintf(&b, "  %s (%s)\n", c.FQN, c.File)
			}
			return fatal(errors.New(strings.TrimRight(b.String(), "\n")))
		}
		return fatal(err)
	}

	gradleArgs := append([]string{module + ":build", "-x", "test"}, args[1:]...)
	return runGradleTask(root, jdk.Home, gradleArgs, alwaysShowGradle, human, color, opts, "COMPILE "+module)
}

func runBuild(args []string, javaMajor int, alwaysShowGradle, human, color bool, opts render.Options) int {
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

	gradleArgs := append([]string{"build"}, args...)
	return runGradleTask(root, jdk.Home, gradleArgs, alwaysShowGradle, human, color, opts, "BUILD")
}

// runGradleTask drives a non-test Gradle task (compile/build). Such tasks may
// still run tests transitively (e.g. "build" depends on "check" -> "test"),
// so a populated Tree is rendered exactly like a normal test run; an empty
// Tree just gets a compact OK/FAILED line, never NOTESTS - zero tests is the
// expected, successful outcome for a plain compile.
func runGradleTask(root, javaHome string, gradleArgs []string, alwaysShowGradle, human, color bool, opts render.Options, label string) int {
	cfg := runner.Config{Root: root, JavaHome: javaHome, Args: gradleArgs, CaptureOutput: opts.ShowOutput}

	var live *render.Live
	if human && isTTY(os.Stdout) {
		live = render.NewLive(os.Stdout, color, 12)
		cfg.OnEvent = live.Handle
	}

	res, err := runner.Execute(cfg)
	if live != nil {
		live.Finish()
	}
	if err != nil {
		return fatal(err)
	}

	switch {
	case res.Tree.Counts().Total > 0:
		if err := writeReport(res.Tree, false, human, color, opts); err != nil {
			return fatal(err)
		}
		recordSavings(root, res.GradleBytes, res.Tree, opts)
	case res.ExitCode == 0:
		fmt.Printf("%s OK\n", label)
	default:
		fmt.Printf("%s FAILED\n", label)
		fmt.Print(indent(reason(res.GradleOutput)))
	}
	if alwaysShowGradle {
		fmt.Print(indent(res.GradleOutput))
	}

	if res.Tree.Counts().Failed > 0 {
		return 1
	}
	if res.ExitCode != 0 {
		if res.Tree.Counts().Total > 0 {
			fmt.Fprint(os.Stderr, indent(reason(res.GradleOutput)))
		}
		return res.ExitCode
	}
	return 0
}

func recordSavings(root string, gradleBytes int64, tree *model.Tree, opts render.Options) {
	actual := agentReportBytes(tree, opts)
	if err := stats.Record(root, gradleBytes, actual); err != nil {
		fmt.Fprintf(os.Stderr, "gtv: stats: %v\n", err)
	}
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

func reason(output string) string {

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

var (
	kotlinError = regexp.MustCompile(`^e: (?:file://)?(\S+?):(\d+(?::\d+)?):?\s*(.*)$`)
	javacError  = regexp.MustCompile(`^(\S+\.java):(\d+):\s*error:\s*(.*)$`)
)

const maxCompileErrors = 10

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
