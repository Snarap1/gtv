# gtv — Gradle test viewer

`gtv` runs `./gradlew test` and shows results the way a person or a coding
agent actually wants them: grouped by `@DisplayName`/`@Nested`, with a
compact machine-readable mode and no scrollback full of Gradle noise.

It reads test events as NDJSON emitted by a Gradle init script
(`internal/gradlescript/listener.gradle`) rather than parsing console
output, so the report is exact even under `--parallel` or a plain
`--console=plain` run.

## Install

Build and install for this machine (Go 1.22+, no external dependencies):

```
./build.sh --install
```

That puts `gtv` in `~/.local/bin` (override with `--prefix=DIR`).
If that directory is not on `PATH`, the script prints the export line to add.

Or build a local binary without installing:

```
go build -o gtv ./cmd/gtv
```

Cross-compile all platforms into `dist/`:

```
./build.sh
# dist/gtv-linux-amd64
# dist/gtv-darwin-arm64
# dist/gtv-windows-amd64.exe
```

The version baked into each binary comes from the nearest git tag
(`git describe --tags --always --dirty`), or `dev` when the repo has none.

### Windows

`build.sh` is a bash script and will not run in `cmd.exe` or PowerShell.
Use WSL, or build natively with plain Go:

```powershell
cd path\to\gradleTestCliViewer
go build -o gtv.exe .\cmd\gtv
```

Requires Go 1.22+ (`go version` to check). This skips the git-tag version
stamping that `build.sh` does, so `gtv --version` reports `dev`.

Put it on `PATH`:

```powershell
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\bin" | Out-Null
Move-Item gtv.exe "$env:USERPROFILE\bin\gtv.exe" -Force
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:USERPROFILE\bin", "User")
```

Open a new terminal for the `PATH` change to take effect, then verify with
`gtv --version`.

Alternatively, cross-compile `dist/gtv-windows-amd64.exe` on Linux/Mac (see
above) and copy that binary over instead of building on Windows at all.

## Uninstall

Remove the installed binary and the cache directory (`~/.cache/gtv`,
or `$XDG_CACHE_HOME/gtv`):

```
./build.sh --uninstall
```

Use the same `--prefix=DIR` you used at install time if it was not the
default. Manually:

```
rm -f ~/.local/bin/gtv
rm -rf ~/.cache/gtv
```

A local `./gtv` binary or the `dist/` cross-builds are not touched;
delete those yourself if you created them.

## Agent skill

`skills/gtv/SKILL.md` teaches a coding agent how to drive `gtv`: target syntax,
the `PASS`/`FAIL`/`NOTESTS` output, exit codes, which flags matter, and when not
to reach for it.
It is a plain `SKILL.md` (the format Claude Code, OpenCode and Codex CLI all read),
so `install-skill.sh` only has to drop it in the right directory:

```
./install-skill.sh              # all three agents, current project
./install-skill.sh --user       # all three agents, this machine
./install-skill.sh --agent=claude --project=/path/to/repo
./install-skill.sh --link       # symlink instead of copy, for editing the skill
./install-skill.sh --uninstall --user
```

| Agent    | Project scope                | User scope                        |
|----------|------------------------------|-----------------------------------|
| Claude Code | `.claude/skills/gtv`      | `~/.claude/skills/gtv`            |
| OpenCode | `.opencode/skills/gtv`       | `~/.config/opencode/skills/gtv`   |
| Codex CLI | `.agents/skills/gtv`        | `~/.codex/skills/gtv`             |

Rerunning is safe: an install created by this script is replaced in place. A
skill directory it did not create is left alone unless you pass `--force`.
`--dry-run` prints the file operations without performing any.
Restart the agent (or open a new session) after installing.

## Usage

```
gtv [flags] <target> [gradle args...]
```

`<target>` is a Gradle task path, a class name/FQN, a path to a test file,
or `Class.method` / `Class::method`.

```
gtv UserServiceTest
gtv UserServiceTest.should pass
gtv :app:service:test --tests "*.UserServiceTest"
gtv :lib:test
```

### Compile and build

`gtv compile <module>` builds one module without running its tests
(`<module>:build -x test`). `<module>` resolves the same way as `<target>`
above (a Gradle module path, class name/FQN, or source file), minus the
`Class.method` part - a compile target names a module, not a test.

`gtv build` runs the whole project's `build` task from the repo root.

```
gtv compile UserServiceTest
gtv compile :app:service
gtv build
```

Both print a compact `COMPILE <module> OK`/`BUILD OK` line on success, and
extract the same trimmed compile-error diagnostics as a failed test run does
on failure - no raw Gradle console noise. If the task happens to run tests
too (e.g. `build` by way of `check`), the usual test tree is rendered instead.

### Flags

| Flag             | Effect                                                            |
|------------------|--------------------------------------------------------------------|
| `--agent`        | force compact agent-oriented output                              |
| `--human`        | force colored tree output                                        |
| `--json`         | print the machine-readable tree instead of human/agent text      |
| `--last`         | read the previous run's JUnit XML reports instead of running Gradle |
| `--watch`        | rerun whenever files under the project change                    |
| `--test-output`  | print captured stdout/stderr of failed tests                     |
| `--max-fail N`   | failures to report in full (0 = all)                              |
| `--java N`       | minimum JDK major version to build with (default 21)             |
| `--no-rerun`     | let Gradle skip UP-TO-DATE or cached test tasks                  |
| `--reindex`      | rebuild the test class index instead of trusting the cache       |
| `--gradle-output`| always print Gradle's own output                                 |
| `--stats`        | print cumulative token-savings stats and exit                    |
| `--version`      | print the gtv version and exit                                   |

Output picks a renderer automatically: a real terminal gets the colored
tree, anything else (a pipe, a coding agent, `CI`/`CLAUDE_CODE` in the
environment) gets the compact agent form. `--agent`/`--human` override
the guess.

After each Gradle run that produced tests, `gtv` records how many tokens
the agent report saved versus the full Gradle console from that same
invocation (`chars/4`), and prints a one-liner to stderr. Totals live in
`~/.cache/gtv/stats.json`; inspect them with `gtv --stats` (or `gtv stats`).

### Sample output

Colored tree (`--human`, or a real terminal):

```
UserQueryService
├─ should return user when found                            ok    12ms
├─ should return empty when not found                        ok     4ms
└─ page=1 size=50 -> expectedLimit=50                        3/3 ok 18ms
```

Compact agent form (default off a terminal):

```
UserServiceTest: 4/4 ok (34ms)
```

A failure keeps the compact form but adds the assertion and a trimmed
stack:

```
UserServiceTest: 3/4 ok, 1 failed (34ms)
  FAILED shouldRejectInvalidId
    expected: <400> but was: <500>
    at com.example.UserServiceTest.shouldRejectInvalidId(UserServiceTest.kt:42)
```

When a report is capped or a run matches nothing, a one-line `hint:` names
the complete next command - the full failure list, a reindex for a freshly
added class, or a rerun without `--last`. Hints go to stderr and never
appear on a fully self-contained report.

When the thrown exception only wraps the real one, the `Caused by` chain is
mined for the link that explains the failure — a Spring context that will not
start names the rejected credential instead of just saying it failed to start:

```
✗ NotionBudgetApplicationTests > contextLoads
  IllegalStateException: Failed to load ApplicationContext for [...]
  caused by: PSQLException: ERROR: password authentication failed for user 'neondb_owner'
```

Links that only requote their own cause are dropped, so the chain usually
collapses to the one line worth reading. `--json` carries the whole chain.

## Windows

Windows support is written but not yet verified live (tracked in
`ROADMAP.md` M5): the wrapper is invoked as `cmd /c gradlew.bat`, and JDK
discovery checks `C:\Program Files\Java`, `C:\Program Files\Eclipse
Adoptium`, `C:\Program Files\Microsoft`, and `%USERPROFILE%\scoop\apps`
ahead of `JAVA_HOME`. The file-watch and event-tailing code polls mtimes
rather than using any OS-specific notification API, so it needs no
Windows-specific path.

## Development

```
go test ./...
```

If the default Go build cache is unavailable:

```
GOCACHE=/tmp/gtv-gocache go test ./...
```

See `ROADMAP.md` for what is implemented and what is still open, and
`CODE_REVIEW.md` for issues already found and fixed.
