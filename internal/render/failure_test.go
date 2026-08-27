package render

import (
	"strings"
	"testing"

	"github.com/pavelnaibich/gtv/internal/event"
)

const kotlinStack = `java.lang.IllegalStateException: boom from probe
	at com.example.service.GtvProbeTest$FailureShapes.helperThrows(GtvProbeTest.kt:52)
	at com.example.service.GtvProbeTest$FailureShapes.should fail with exception(GtvProbeTest.kt:38)
	at java.base/java.lang.reflect.Method.invoke(Method.java:580)
	at java.base/java.util.ArrayList.forEach(ArrayList.java:1596)`

func TestFramesKeepsUserCodeOnly(t *testing.T) {
	got := Frames(kotlinStack, 3)
	want := []string{"GtvProbeTest.kt:52", "GtvProbeTest.kt:38"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Frames = %v, want %v", got, want)
	}
}

func TestFramesRespectsMax(t *testing.T) {
	if got := Frames(kotlinStack, 1); len(got) != 1 {
		t.Fatalf("Frames with max 1 = %v", got)
	}
}

func TestHeadlinePrefixesClassOnce(t *testing.T) {
	f := event.Fail{Cls: "java.lang.AssertionError", Msg: "\nExpected size: 5 but was: 3 in:\n[1, 2, 3]"}
	got := Headline(f, 6)
	want := []string{"AssertionError: Expected size: 5 but was: 3 in:", "[1, 2, 3]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
}

func TestHeadlineTruncates(t *testing.T) {
	f := event.Fail{Cls: "X", Msg: "a\nb\nc\nd"}
	got := Headline(f, 2)
	if len(got) != 3 || got[2] != "…" {
		t.Fatalf("Headline = %q", got)
	}
}

func TestWantsDiffSuppressesRedundantPair(t *testing.T) {
	redundant := event.Fail{Msg: "expected: <42> but was: <7>", Expected: "42", Actual: "7"}
	if wantsDiff(redundant) {
		t.Error("expected/actual already in message, should not be repeated")
	}
	useful := event.Fail{Msg: "arrays differ", Expected: "[1, 2]", Actual: "[1, 3]"}
	if !wantsDiff(useful) {
		t.Error("expected/actual carry new information, should be printed")
	}
}

const springChain = `java.lang.IllegalStateException: Failed to load ApplicationContext
	at org.springframework.test.context.cache.DefaultCacheAwareContextLoaderDelegate.loadContext(DefaultCacheAwareContextLoaderDelegate.java:180)
Caused by: org.springframework.beans.factory.BeanCreationException: Error creating bean with name 'entityManagerFactory'
	at org.springframework.beans.factory.support.AbstractBeanFactory.doGetBean(AbstractBeanFactory.java:333)
Caused by: org.springframework.beans.factory.BeanCreationException: Error creating bean with name 'liquibase'
	... 42 more
Caused by: liquibase.exception.UnexpectedLiquibaseException: liquibase.exception.DatabaseException: org.postgresql.util.PSQLException: ERROR: password authentication failed for user 'neondb_owner'
Caused by: liquibase.exception.DatabaseException: org.postgresql.util.PSQLException: ERROR: password authentication failed for user 'neondb_owner'
Caused by: org.postgresql.util.PSQLException: ERROR: password authentication failed for user 'neondb_owner'
	at org.postgresql.core.v3.ConnectionFactoryImpl.doAuthentication(ConnectionFactoryImpl.java:693)
`

func TestCausesKeepsRootAndDropsWrappers(t *testing.T) {
	got := Causes(springChain, 0)
	want := []string{
		"BeanCreationException: Error creating bean with name 'entityManagerFactory'",
		"BeanCreationException: Error creating bean with name 'liquibase'",
		"PSQLException: ERROR: password authentication failed for user 'neondb_owner'",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Causes = %q, want %q", got, want)
	}
}

func TestCausesCapKeepsTheRoot(t *testing.T) {
	got := Causes(springChain, 1)
	want := []string{"PSQLException: ERROR: password authentication failed for user 'neondb_owner'"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Causes = %q, want %q", got, want)
	}

	got = Causes(springChain, 2)
	if len(got) != 2 || got[1] != want[0] {
		t.Fatalf("Causes = %q, want the root last", got)
	}
}

func TestCausesWithoutAMessage(t *testing.T) {
	stack := "boom\nCaused by: java.lang.NullPointerException\n\tat com.example.Foo.bar(Foo.kt:12)"
	got := Causes(stack, 0)
	if len(got) != 1 || got[0] != "NullPointerException" {
		t.Fatalf("Causes = %q, want [NullPointerException]", got)
	}
}

func TestCausesIgnoresFramesAndElisions(t *testing.T) {
	stack := "org.opentest4j.AssertionFailedError: expected: <4> but was: <1>\n\tat com.example.FooTest.bar(FooTest.kt:71)\n\t... 42 more"
	if got := Causes(stack, 0); got != nil {
		t.Fatalf("Causes = %q, want none", got)
	}
	if got := Causes("", 0); got != nil {
		t.Fatalf("Causes = %q, want none", got)
	}
}
