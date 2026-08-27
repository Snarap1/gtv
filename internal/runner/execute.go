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

type Config struct {
	Root     string
	JavaHome string
	Args     []string

	ForceRerun bool

	OnEvent func(event.Event)

	CaptureOutput bool
}

type Result struct {
	Tree     *model.Tree
	ExitCode int

	GradleOutput string

	GradleBytes int64
	Events      int
}

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
