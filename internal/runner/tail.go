package runner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pavelnaibich/gtv/internal/event"
)

const pollInterval = 40 * time.Millisecond

func tailEvents(path string, stop <-chan struct{}, out chan<- event.Event) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening event stream: %w", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64*1024)
	var partial []byte

	drain := func(final bool) error {
		for {
			chunk, readErr := r.ReadBytes('\n')
			if len(chunk) > 0 {
				partial = append(partial, chunk...)
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					return fmt.Errorf("reading event stream: %w", readErr)
				}
				if final && len(bytes.TrimSpace(partial)) > 0 {
					return fmt.Errorf("malformed event stream: unterminated final line")
				}
				return nil
			}
			line := partial
			partial = nil
			if e, ok := event.Decode(line); ok {
				out <- e
			} else if len(bytes.TrimSpace(line)) > 0 {
				return fmt.Errorf("malformed event stream: unreadable NDJSON line")
			}
		}
	}

	for {
		if err := drain(false); err != nil {
			return err
		}
		select {
		case <-stop:
			if err := drain(true); err != nil {
				return err
			}
			return nil
		case <-time.After(pollInterval):
		}
	}
}
