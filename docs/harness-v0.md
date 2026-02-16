# yoke Harness v0

This is the original v0 architecture note. For current operational docs, use:
- `/Users/pealco/archive/yoke/docs/quickstart.md`
- `/Users/pealco/archive/yoke/docs/command-reference.md`
- `/Users/pealco/archive/yoke/docs/how-it-works.md`

## Objective

Run a solo two-agent workflow where one session writes code and another session reviews it, with `td` as the source of truth and PRs as the merge boundary.

## Components

- `td`: task lifecycle (`open -> in_progress -> in_review -> closed`)
- `git` branch per issue: `yoke/<td-id>`
- `gh` draft PR per issue branch
- `cmd/yoke/main.go`: Go command harness implementation
- `bin/yoke`: launcher that runs `go run ./cmd/yoke`

## Workflow

1. Claim issue:
   - `yoke claim [td-id]`
   - Starts a fresh td session.
   - Moves issue to `in_progress`.
   - Switches to `yoke/<td-id>` branch.

Initialization:
- `yoke init` auto-detects installed agents (`codex`, `claude`) and prompts for writer/reviewer.
- Writer and reviewer may be the same agent.
- Selections are saved in `.yoke/config.sh` (`YOKE_WRITER_AGENT`, `YOKE_REVIEWER_AGENT`).

2. Write and commit:
   - Writer agent implements changes.
   - Writer runs checks.

3. Submit for review:
   - `yoke submit [td-id] --done "..." --remaining "..."`
   - Runs checks (`.yoke/checks.sh` by default).
   - Writes `td handoff` fields.
   - Moves issue to `in_review`.
   - Pushes branch and opens draft PR.

4. Review from separate session:
   - `yoke review [td-id] --agent` (optional automated reviewer)
   - `yoke review [td-id] --approve` to close.
   - `yoke review [td-id] --reject "reason"` to send back.

## Command Reference

```bash
yoke init [--writer-agent codex|claude] [--reviewer-agent codex|claude] [--td-prefix PREFIX] [--no-prompt]
yoke doctor
yoke status
yoke claim [td-id]
yoke submit [td-id] --done "..." --remaining "..." [--decision "..."] [--uncertain "..."]
yoke review [td-id] [--agent] [--note "..."] [--approve | --reject "..."]
yoke help [command]
```

## Configuration

Edit `.yoke/config.sh`:

- `YOKE_BASE_BRANCH`: PR base branch.
- `YOKE_CHECK_CMD`: quality gate command/path.
- `YOKE_REVIEW_CMD`: optional reviewer automation command.
- `YOKE_PR_TEMPLATE`: PR template path.

## Build and Distribution

- Local build: `make build` (outputs `dist/yoke`)
- Local install: `make install`
- Cross-platform binaries: `make release` (macOS/Linux, `amd64` + `arm64`)

## Quality Gates

- Formatter: `make fmt` / `make fmt-check`
- Lint: `make lint`
- Type checking: `make typecheck`
- Vet: `make vet`
- Tests: `make test` (optional race detector: `make test-race`)
- Vulnerability scan: `make vuln`
- Aggregate gate: `make check`
- Harness integration: `.yoke/checks.sh` runs `make check` during `yoke submit`
