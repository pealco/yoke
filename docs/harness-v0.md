# yoke Harness v0

This is the original v0 architecture note. For current operational docs, use:
- `/Users/pealco/archive/yoke/docs/quickstart.md`
- `/Users/pealco/archive/yoke/docs/command-reference.md`
- `/Users/pealco/archive/yoke/docs/how-it-works.md`

## Objective

Run a solo two-agent workflow where one session writes code and another session reviews it, with `bd` as the source of truth and PRs as the merge boundary.

## Components

- `bd`: task lifecycle (`open -> in_progress -> blocked+yoke:in_review -> closed`)
- `git` branch per issue: `yoke/<bd-id>`
- `gh` draft PR per issue branch
- `cmd/yoke/main.go`: Go command harness implementation
- `bin/yoke`: launcher that runs `go run ./cmd/yoke`

## Workflow

1. Claim issue:
   - `yoke claim [bd-id]`
   - Moves issue to `in_progress` and clears `yoke:in_review` label.
   - Switches to `yoke/<bd-id>` branch.

Initialization:
- `yoke init` auto-detects installed agents (`codex`, `claude`) and prompts for writer/reviewer.
- Writer and reviewer may be the same agent.
- Selections are saved in `.yoke/config.sh` (`YOKE_WRITER_AGENT`, `YOKE_REVIEWER_AGENT`).

2. Write and commit:
   - Writer agent implements changes.
   - Writer runs checks.

3. Submit for review:
   - `yoke submit [bd-id] --done "..." --remaining "..."`
   - Runs checks (`.yoke/checks.sh` by default).
   - Writes handoff details as `bd comments add`.
   - Moves issue to review queue (`blocked` + `yoke:in_review`).
   - Pushes branch and opens draft PR.
   - Posts writer handoff comment to PR.

4. Review from separate session:
   - `yoke review [bd-id] --agent` (optional automated reviewer)
   - `yoke review [bd-id] --approve` to close.
   - `yoke review [bd-id] --reject "reason"` to send back.
   - Posts reviewer decision/note comments to PR.
   - Approval lifts PR draft status to ready-for-review.

## Command Reference

```bash
yoke init [--writer-agent codex|claude] [--reviewer-agent codex|claude] [--bd-prefix PREFIX] [--no-prompt]
yoke doctor
yoke status
yoke daemon [--once] [--interval VALUE] [--max-iterations N]
yoke claim [bd-id]
yoke submit [bd-id] --done "..." --remaining "..." [--decision "..."] [--uncertain "..."]
yoke review [bd-id] [--agent] [--note "..."] [--approve | --reject "..."]
yoke help [command]
```

## Configuration

Edit `.yoke/config.sh`:

- `YOKE_BASE_BRANCH`: PR base branch.
- `YOKE_CHECK_CMD`: quality gate command/path.
- `YOKE_WRITER_CMD`: optional writer automation command (daemon loop).
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
