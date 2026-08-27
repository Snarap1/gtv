package event

import (
	"bytes"
	"encoding/json"
)

const (
	SuiteStart = "suiteStart"
	SuiteEnd   = "suiteEnd"
	TestStart  = "testStart"
	TestEnd    = "testEnd"
	Output     = "out"
)

const (
	Success = "SUCCESS"
	Failure = "FAILURE"
	Skipped = "SKIPPED"
)

type Fail struct {
	Msg   string `json:"msg"`
	Cls   string `json:"cls"`
	Stack string `json:"stack"`

	Assertion  bool `json:"assertion"`
	Assumption bool `json:"assumption"`

	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

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
