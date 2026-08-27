package main

import "testing"

// Captured from Gradle 9.2 + Kotlin 2.x: the failure section says nothing useful,
// the diagnostic sits above it.
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
