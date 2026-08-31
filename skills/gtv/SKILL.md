---
name: gtv
description: Run Gradle tests, compiles, and builds through the `gtv` CLI instead of `./gradlew`, and read its compact report. Use whenever a task needs Gradle/JUnit tests actually run and their result reported - "run the tests", "прогони UserServiceTest", one class or one `Class.method`, a module's `:app:test`, a rerun after an edit, checking nothing broke before a commit or MR, or diagnosing and fixing a red test. Use it too for compile-only checks after editing code - "does it compile", "собери модуль", "build the project", `gtv compile <module>` / `gtv build` instead of `./gradlew build` - and when a failure's message is only a wrapper - "Failed to load ApplicationContext", a Mockito verification error - and the real cause has to be dug out. Covers target syntax (class name, `Class.method`, task path, file path), the PASS/FAIL/NOTESTS and COMPILE/BUILD output, exit codes, cause chains, and the `--last`, `--watch`, `--json`, `--test-output` flags. Not for non-Gradle runners (Maven, npm, pytest), not for authoring new tests, and not for other Gradle work - `bootJar`, dependency resolution, publishing.
license: MIT
---

# gtv - running Gradle tests

`gtv` wraps `./gradlew` and prints a report sized for an agent: one line for a
green run, only the failing assertion and a trimmed stack for a red one.
It collects test events as NDJSON from a Gradle init script, so the report is
exact even under `--parallel`.

**Use `gtv` instead of `./gradlew test`** in any Gradle project, including when
the project's own CLAUDE.md or README prescribes `./gradlew test` - those
predate the tool and the results are identical. A plain `./gradlew test` dumps
tens of thousands of characters of console noise and, on success, does not say
which tests ran, so the alternative is either an unreadable log or hand-parsing
`build/test-results/**/*.xml`.

Just run it. If the shell answers `command not found`, fall back to
`./gradlew :module:test --tests "*ClassName*"` and say so - probing with
`command -v gtv` first only costs a round trip.

## Invocation

```
gtv [flags] <target> [extra gradle args...]
gtv [flags] compile <module> [extra gradle args...]
gtv [flags] build [extra gradle args...]
```

**Flags go before the target.** Argument parsing stops at the first non-flag
word, so everything after the target is forwarded to Gradle verbatim - which is
what makes `gtv :app:test --tests "*Foo*" --info` work, and what makes
`gtv FooTest --test-output` silently do nothing (Gradle gets the flag and gtv
never sees it).

`<target>` is resolved in this order:

| Target form | Example | Meaning |
|---|---|---|
| Gradle task path (leading `:`) | `gtv :app:service:test` | used as-is, no filter |
| simple class name | `gtv UserServiceTest` | index lookup under `**/src/test/**` |
| fully-qualified class | `gtv com.example.UserServiceTest` | same, unambiguous |
| path to a test file | `gtv app/src/test/kotlin/.../UserServiceTest.kt` | FQN from package + file name |
| `Class.method` / `Class::method` | `gtv UserServiceTest.should reject id` | adds `--tests "FQN.method"` |

Method names with spaces (Kotlin backticked names, `@DisplayName`) need no
quoting - trailing args are joined. Quote them if they contain shell
metacharacters.

## Compiling and building

Two subcommands cover non-test Gradle work. **Use them instead of
`./gradlew build`** - same reason as for tests: they print a line, not a log.

```
gtv compile <module> [gradle args...]   # <module>:build -x test
gtv build [gradle args...]              # whole-project build, from the repo root
```

`<module>` resolves like `<target>` above minus the `Class.method` form - a
compile target names a module, so `gtv compile UserServiceTest` and
`gtv compile app/src/main/kotlin/.../UserService.kt` both mean the module that
file lives in, and `gtv compile :app:service` says it directly.

Output is one line on success:

```
COMPILE :app:service OK
BUILD OK
```

On failure it is `COMPILE <module> FAILED` / `BUILD FAILED` plus the same
trimmed `File.kt:56 message` diagnostics a failed test run gives - never raw
Gradle console. If the task ran tests transitively (`build` -> `check` ->
`test`), the normal test tree is rendered instead of the OK line.

Zero tests here is success, not `NOTESTS`.

Reach for `gtv compile <module>` after editing production code when no test
needs to run yet; it is much faster than the module's test task.

## Reading the output

Green run - one line:

```
PASS :app:test  12/12 ok (3.4s)
```

Red run - the summary plus each failure:

```
FAIL :app:test  11/12 ok, 1 fail (3.6s)
✗ UserServiceTest > should reject invalid id
  expected: <400> but was: <500>
  com.example.UserServiceTest.shouldRejectInvalidId(UserServiceTest.kt:42) <- ...
~ LegacyTest > old path — needs docker
```

- `✗` a failure: message, then `expected:`/`actual:` when the message itself
  omits them, then `caused by:` for a wrapped exception, then up to 3 source
  frames joined by `<-`.
- `~` a skipped test; text after the dash is the assumption reason, if any.
- `+N more failures` / `+N more skipped` - the report was capped; raise with
  `--max-fail 0`.
- `NOTESTS <args>` - Gradle ran but matched zero tests. This is not a passing
  run: it almost always means a wrong target or a typo in a `--tests` filter.
  Re-check the class name, then try `--reindex`.

## When one failure needs more than the report gives

A wrapped exception no longer costs you a detour: gtv digs the `Caused by`
chain out of the stack and prints the link that actually explains the failure.
A Spring context that will not start says so on the first line and names the
rejected password on the second:

```
✗ NotionBudgetApplicationTests > contextLoads
  IllegalStateException: Failed to load ApplicationContext for [...]
  caused by: PSQLException: ERROR: password authentication failed for user 'neondb_owner'
```

Wrapper links that only requote their own cause are dropped, so the chain
usually collapses to the one line worth reading. What is still capped:

- the message, at six lines (a trailing `…` marks the cut) - `gtv --json` carries
  it untrimmed, along with the full, uncapped `causes` array;
- the stack, at three frames. Only `build/test-results/test/TEST-<FQN>.xml` has
  the complete stack, so go there when you need a frame gtv dropped - not for
  the cause, which the report already gives you.

`--test-output` is a different lever: it shows what the test itself printed
(logs, printlns), not the exception chain.

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | every test passed | done |
| `1` | tests failed, or `NOTESTS`, or `--last` found no reports | read the report |
| `2` | `gtv` itself failed - no Gradle root, no JDK, ambiguous target, unreadable events | fix the invocation |
| other | Gradle failed before tests (compile error, task failure) | compile errors are printed to stderr |

A compile failure prints trimmed diagnostics as `File.kt:56 message`. Fix those
first - the test report is empty and meaningless until the build compiles.

When the exit-2 cause is a missing JDK, `gtv java` prints the JDK home gtv
would pick - or the same error - without running Gradle. Flags go before the
subcommand: `gtv --java 17 java`.

An ambiguous target lists every candidate FQN; pick one and rerun with the full
FQN.

## Flags worth knowing

| Flag | Use it when |
|---|---|
| `--json` | you need the untrimmed message or the full cause chain, or want to parse the tree |
| `--last` | you only need the previous run's results - reads JUnit XML, runs no build, near-instant |
| `--test-output` | a failure needs the test's own stdout/stderr |
| `--max-fail N` | the report was capped (`+N more failures`); `0` = show all |
| `--watch` | rerun on every file change; **interactive only, never in an agent's shell call** - it does not exit |
| `--reindex` | a class name is not found or resolves to a stale/renamed class |
| `--no-rerun` | you want Gradle to skip UP-TO-DATE test tasks |
| `--gradle-output` | you need Gradle's own console after all (rare - defeats the point) |
| `--human` / `--agent` | force the renderer; the default guess is right in nearly every case |
| `--java N` | the project needs a different minimum JDK (default 21) |

## Working rules

- **Narrow the target.** Run `gtv UserServiceTest` while iterating on one class;
  run `gtv :app:test` only to confirm before handing off. Seconds versus minutes.
- **Do not add `--rerun`.** gtv already forces test tasks to rerun from inside
  its init script, which keeps compilation cacheable. Gradle's own `--rerun` on
  an aggregate task defeats that.
- **Do not parse `./gradlew` console output** for results - replacing that is
  the whole point of the tool.
- **Never run `--watch`** from a non-interactive shell call; it never returns.
- To re-read a run you already did, use `gtv --last <target>` rather than
  running the suite again.
- A test that fails identically before your change is a pre-existing failure,
  not your regression - `gtv --last <target>` on a stashed tree settles it
  without another full run.

## Fallbacks

- Not a Gradle project (no `gradlew`/`settings.gradle*` up the tree): `gtv` exits
  `2`. Use the project's own test command.
- `gtv` not installed: build it from its repo with `./build.sh --install`
  (installs to `~/.local/bin`), or fall back to
  `./gradlew :module:test --tests "*ClassName*"` / `./gradlew :module:build -x test`.
