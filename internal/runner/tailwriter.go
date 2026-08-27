package runner

// maxGradleLog bounds the Gradle log kept in memory. Only the tail is ever
// shown, and a --info run can produce hundreds of megabytes.
const maxGradleLog = 256 * 1024

// tailWriter keeps the last maxGradleLog bytes written to it and discards the rest.
// Total counts every byte accepted, including those later dropped from the buffer,
// so callers can measure the true Gradle console size without retaining it.
type tailWriter struct {
	buf     []byte
	dropped bool
	Total   int64
}

func (t *tailWriter) Write(p []byte) (int, error) {
	n := len(p)
	t.Total += int64(n)
	if n >= maxGradleLog {
		t.buf = append(t.buf[:0], p[n-maxGradleLog:]...)
		t.dropped = true
		return n, nil
	}
	if len(t.buf)+n > maxGradleLog {
		drop := len(t.buf) + n - maxGradleLog
		t.buf = append(t.buf[:0], t.buf[drop:]...)
		t.dropped = true
	}
	t.buf = append(t.buf, p...)
	return n, nil
}

// String returns the retained tail, marked when earlier output was dropped.
func (t *tailWriter) String() string {
	if t.dropped {
		return "[…earlier output dropped…]\n" + string(t.buf)
	}
	return string(t.buf)
}
