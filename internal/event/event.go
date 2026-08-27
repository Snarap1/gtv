// Package event decodes the NDJSON stream produced by the gtv Gradle init script.
package event

import (
	"bytes"
	"encoding/json"
)

// Event kinds emitted by listener.gradle.
const (
	SuiteStart = "suiteStart"
	SuiteEnd   = "suiteEnd"
	TestStart  = "testStart"
	TestEnd    = "testEnd"
	Output     = "out"
)

// Result types mirror org.gradle.api.tasks.testing.TestResult.ResultType.
const (
	Success = "SUCCESS"
	Failure = "FAILURE"
	Skipped = "SKIPPED"
)

// Fail is one failure attached to a test or suite.
type Fail struct {
	Msg   string `json:"msg"`
	Cls   string `json:"cls"`
	Stack string `json:"stack"`
	// Assertion is true when the framework reported an assertion failure rather
	// than an unexpected exception.
	Assertion  bool `json:"assertion"`
	Assumption bool `json:"assumption"`
	// Expected and Actual are only populated for opentest4j failures (JUnit's
	// assertEquals); AssertJ throws a plain AssertionError and leaves them empty.
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Event is one line of the NDJSON stream.
type Event struct {
	E    string `json:"e"`
	Task string `json:"task"`

	Key     string `json:"key"`
	Parent  string `json:"parent"`
	Name    string `json:"name"`
	Display string `json:"display"`
	Cls     string `json:"cls"`

	Res   string `json:"res"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`

	Total   int64 `json:"total"`
	Ok      int64 `json:"ok"`
	Failed  int64 `json:"failed"`
	SkipCnt int64 `json:"skipped"`

	Failures []Fail `json:"failures"`
	Assumed  *Fail  `json:"assumed"`

	Dst string `json:"dst"`
	Msg string `json:"msg"`
}

// Decode reads one NDJSON line. Blank lines yield ok=false.
func Decode(line []byte) (Event, bool) {
	if len(bytes.TrimSpace(line)) == 0 {
		return Event{}, false
	}
	var e Event
	if err := json.Unmarshal(line, &e); err != nil {
		return Event{}, false
	}
	if e.Key == "" || e.Task == "" {
		return Event{}, false
	}
	switch e.E {
	case SuiteStart, SuiteEnd, TestStart, TestEnd, Output:
		return e, true
	default:
		return Event{}, false
	}
}
