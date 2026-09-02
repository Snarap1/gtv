package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

const kotlinCompileLog = `> Task :m:compileTestKotlin FAILED
e: file:///home/u/repo/src/test/kotlin/com/mm/GtvProbeTest.kt:50:33 Return type mismatch: expected 'Int', actual 'String'.

FAILURE: Build failed with an exception.

* What went wrong:
Execution failed for task ':m:compileTestKotlin'.
> A failure occurred while executing org.jetbrains.kotlin.compilerRunner.GradleCompilerRunnerWithWorkers
   > Compilation error. See log for more details

* Try:
> Run with --stacktrace option to get the stack trace.
`

func TestReasonPrefersCompileDiagnostics(t *testing.T) {
	want := "GtvProbeTest.kt:50:33 Return type mismatch: expected 'Int', actual 'String'."
	if got := reason(kotlinCompileLog); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

const noTestsLog = `> Task :m:test FAILED

FAILURE: Build failed with an exception.

* What went wrong:
Execution failed for task ':m:test'.
> No tests found for given includes: [com.mm.NoSuchTest](--tests filter)

* Try:
> Run with --stacktrace option to get the stack trace.
`

func TestReasonExtractsWhatWentWrong(t *testing.T) {
	want := "Execution failed for task ':m:test'.\nNo tests found for given includes: [com.mm.NoSuchTest](--tests filter)"
	if got := reason(noTestsLog); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

func TestReasonFallsBackToTail(t *testing.T) {
	if got := reason("something died\nwithout a section\n"); got != "something died\nwithout a section" {
		t.Fatalf("reason = %q", got)
	}
}

func TestCompileErrorsHandlesJavac(t *testing.T) {
	log := "/home/u/repo/src/main/java/com/mm/Foo.java:12: error: cannot find symbol\n"
	got := compileErrors(log)
	if len(got) != 1 || got[0] != "Foo.java:12 cannot find symbol" {
		t.Fatalf("compileErrors = %q", got)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestHintsAreCompleteCommands(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
		want string
	}{
		{
			name: "resolveTargetHint",
			fn:   func() { resolveTargetHint("UserServiceTest") },
			want: "hint: check the target; if the class is new or renamed, run gtv --reindex UserServiceTest\n",
		},
		{
			name: "noResultsHint",
			fn:   func() { noResultsHint(":m:test") },
			want: "hint: run gtv :m:test (without --last) to produce results\n",
		},
		{
			name: "noTestsHint",
			fn:   func() { noTestsHint(":m:test") },
			want: "hint: check the target/filter; for a newly added class run gtv --reindex :m:test\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureStderr(t, tc.fn); got != tc.want {
				t.Errorf("hint = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHintsGoToStderr(t *testing.T) {
	got := captureStderr(t, func() { noResultsHint(":m:test") })
	if !strings.HasPrefix(got, "hint: ") {
		t.Errorf("hint = %q, want stderr output with \"hint: \" prefix", got)
	}
}
