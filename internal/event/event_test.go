package event

import "testing"

func TestDecodeRejectsIncompleteOrUnknownEvent(t *testing.T) {
	for _, line := range []string{
		`{}`,
		`{"e":"unknown","task":":test","key":"1"}`,
		`{"e":"testStart","task":"","key":"1"}`,
	} {
		if _, ok := Decode([]byte(line)); ok {
			t.Errorf("Decode(%s) succeeded, want rejection", line)
		}
	}
}
