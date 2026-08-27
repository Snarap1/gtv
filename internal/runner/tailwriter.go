package runner

const maxGradleLog = 256 * 1024

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

func (t *tailWriter) String() string {
	if t.dropped {
		return "[…earlier output dropped…]\n" + string(t.buf)
	}
	return string(t.buf)
}
