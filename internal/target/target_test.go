package target

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func buildFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, ".cache"))

	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("build.gradle", "")
	write("src/test/java/RootTest.java", "class RootTest {}\n")

	write("moduleA/build.gradle", "")
	write("moduleA/src/test/java/com/example/FooTest.java", "package com.example;\nclass FooTest {}\n")

	write("moduleB/build.gradle.kts", "")
	write("moduleB/src/test/kotlin/com/other/FooTest.kt", "package com.other\nclass FooTest\n")

	write("moduleC/build.gradle", "")
	write("moduleC/src/test/java/com/example/BarTest.java", "package com.example;\nclass BarTest {}\n")

	return root
}

func TestResolveTaskPathAsIs(t *testing.T) {
	root := buildFixture(t)
	got, cands, err := Resolve(root, ":a:b:test", false)
	if err != nil {
		t.Fatal(err)
	}
	if cands != nil {
		t.Fatalf("candidates = %v, want nil", cands)
	}
	want := Target{Task: ":a:b:test"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveSourceFilePath(t *testing.T) {
	root := buildFixture(t)
	arg := filepath.Join(root, "moduleA", "src", "test", "java", "com", "example", "FooTest.java")
	got, _, err := Resolve(root, arg, false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleA:test", TestFilter: "com.example.FooTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveSimpleClassNameUnique(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "BarTest", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.example.BarTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveFQN(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "com.other.FooTest", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleB:test", TestFilter: "com.other.FooTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveClassAndMethodWithSpace(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "BarTest.should pass", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.example.BarTest.should pass"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveFQNAndMethod(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "com.example.BarTest.should pass", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.example.BarTest.should pass"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveClassAndMethodDoubleColon(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "BarTest::should_pass", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.example.BarTest.should_pass"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveRootModule(t *testing.T) {
	root := buildFixture(t)
	got, _, err := Resolve(root, "RootTest", false)
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Task: ":test", TestFilter: "RootTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	root := buildFixture(t)
	_, cands, err := Resolve(root, "FooTest", false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %v, want 2", cands)
	}
}

func TestResolveNotFound(t *testing.T) {
	root := buildFixture(t)
	_, _, err := Resolve(root, "NoSuchTest", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestIndexCachePicksUpNewFile(t *testing.T) {
	root := buildFixture(t)

	if _, _, err := Resolve(root, "BarTest", false); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)

	newFile := filepath.Join(root, "moduleC", "src", "test", "java", "com", "example", "BazTest.java")
	if err := os.WriteFile(newFile, []byte("package com.example;\nclass BazTest {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := Resolve(root, "BazTest", false)
	if err != nil {
		t.Fatalf("BazTest not picked up by cache refresh: %v", err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.example.BazTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestIndexCachePicksUpEditedPackage(t *testing.T) {
	root := buildFixture(t)
	if _, _, err := Resolve(root, "com.example.BarTest", false); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	file := filepath.Join(root, "moduleC", "src", "test", "java", "com", "example", "BarTest.java")
	if err := os.WriteFile(file, []byte("package com.renamed;\nclass BarTest {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := Resolve(root, "com.renamed.BarTest", false)
	if err != nil {
		t.Fatalf("edited package not picked up by cache refresh: %v", err)
	}
	want := Target{Task: ":moduleC:test", TestFilter: "com.renamed.BarTest"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}
