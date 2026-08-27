package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pavelnaibich/gtv/internal/event"
	"github.com/pavelnaibich/gtv/internal/gradlescript"
	"github.com/pavelnaibich/gtv/internal/model"
)

// Config describes one Gradle invocation.
type Config struct {
	Root     string   // directory holding the Gradle wrapper
	JavaHome string   // JDK to build with
	Args     []string // task paths and Gradle flags, e.g. [":a:b:test", "--tests", "X"]
	// ForceRerun makes every Test task ignore up-to-date checks and the build
	// cache. Gradle's own --rerun cannot do this: it is a task option, so on an
	// aggregate task like `check` it reruns the aggregate and leaves the Test
	// task UP-TO-DATE, producing a run with no events at all.
	ForceRerun bool
	// OnEvent, when set, is called for every event as it arrives, on a single
	// goroutine, before the run finishes.
	OnEvent func(event.Event)
	// CaptureOutput asks the init script to stream test stdout/stderr. It is
	// disabled by default because retaining output is only useful for the
	// --test-output report and can be voluminous.
	CaptureOutput bool
}

// Result is what a finished run produced.
type Result struct {
	Tree     *model.Tree
	ExitCode int
	// GradleOutput is the wrapper's own stdout+stderr, truncated to its tail and
	// kept back unless something went wrong outside the tests themselves (a
	// compile error, no matching test).
	GradleOutput string
	// GradleBytes is the full size of that stdout+stderr stream, including bytes
	// discarded from the retained tail. Used as the token-savings baseline.
	GradleBytes int64
	Events      int
}

// Execute runs Gradle with the listener attached and folds the streamed events
// into a tree.
func Execute(cfg Config) (*Result, error) {
	tmp, err := os.MkdirTemp("", "gtv-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	script, err := gradlescript.Materialize(tmp)
	if err != nil {
		return nil, err
	}
	ndjson := filepath.Join(tmp, "events.ndjson")
	if err := os.WriteFile(ndjson, nil, 0o644); err != nil {
		return nil, err
	}

	name, lead := wrapperCommand(cfg.Root)
	args := append(append([]string{}, lead...), cfg.Args...)
	args = append(args, "--console=plain", "-I", script, "-Dgtv.out="+ndjson)
	if cfg.ForceRerun {
		args = append(args, "-Dgtv.rerun=true")
	}
	if cfg.CaptureOutput {
		args = append(args, "-Dgtv.output=true")
	}

	var output tailWriter
	cmd := exec.Command(name, args...)
	cmd.Dir = cfg.Root
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append(os.Environ(), "JAVA_HOME="+cfg.JavaHome)

	tree := model.New()
	events := make(chan event.Event, 256)
	stop := make(chan struct{})
	tailDone := make(chan struct{})
	applyDone := make(chan struct{})
	count := 0

	var tailErr error
	go func() {
		defer close(tailDone)
		defer close(events)
		tailErr = tailEvents(ndjson, stop, events)
	}()
	go func() {
		defer close(applyDone)
		for e := range events {
			count++
			tree.Apply(e)
			if cfg.OnEvent != nil {
				cfg.OnEvent(e)
			}
		}
	}()

	runErr := cmd.Run()
	close(stop)
	<-tailDone
	<-applyDone

	if tailErr != nil {
		return nil, tailErr
	}

	res := &Result{
		Tree:         tree,
		GradleOutput: output.String(),
		GradleBytes:  output.Total,
		Events:       count,
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	} else if runErr != nil {
		return nil, fmt.Errorf("running gradle: %w", runErr)
	}
	return res, nil
}
