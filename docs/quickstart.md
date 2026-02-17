# Quickstart

This guide gets you from zero to a complete `claim -> submit -> review` cycle.

## 1. Prerequisites

Install and verify:

```bash
go version
git --version
bd --version
gh --version
```

Notes:
- `bd` is required for core workflow.
- `gh` is optional; if missing, PR creation is skipped.

## 2. Build or install yoke

```bash
# from repository root
make build

# optional global install
make install
```

You can run via:
- `./bin/yoke ...` (repository-local launcher)
- `yoke ...` (if installed globally)

## 3. Initialize

```bash
./bin/yoke init
```

During `init`, yoke:
1. Ensures scaffold folders exist.
2. Autodetects installed coding agents (`codex`, `claude`/`claude-code`).
3. Prompts for the `bd` issue prefix (default `bd`).
4. Prompts you to choose writer and reviewer agents.
5. Saves choices in `.yoke/config.sh`.

Non-interactive setup:

```bash
./bin/yoke init --no-prompt --writer-agent codex --reviewer-agent claude

# explicitly set bd issue prefix
./bin/yoke init --no-prompt --bd-prefix bd --writer-agent codex --reviewer-agent claude
```

Using the same agent for both roles is supported:

```bash
./bin/yoke init --no-prompt --writer-agent codex --reviewer-agent codex
```

## 4. Validate environment

```bash
./bin/yoke doctor
```

Expected checks:
- `git` present
- `bd` present
- `gh` presence (warning only)
- config file exists
- configured bd prefix
- configured writer/reviewer agent availability

## 5. Run one task end-to-end

### Inspect status snapshot

```bash
./bin/yoke status
```

Use this before `claim`, `submit`, or `review` to verify branch, configured agents, and `bd` focus/next context.

### Configure daemon commands (optional but recommended)

Set writer/reviewer commands in `.yoke/config.sh`:

```bash
YOKE_WRITER_CMD='codex exec "Implement $ISSUE_ID, commit, then run yoke submit $ISSUE_ID --done \"...\" --remaining \"...\""'
YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
```

Then run:

```bash
./bin/yoke daemon --once
# remove --once for continuous loop mode
```

### Claim

```bash
./bin/yoke claim
```

Behavior:
- picks issue from `bd list --status open --ready` (if not provided)
- if the issue is an epic, runs a five-pass improvement cycle with alternating writer/reviewer agents
- records pass reports and summary in `.yoke/epic-improvement-reports/<epic-id>/`
- if the issue is an epic, resolves to the next child task (`in_progress` first, then ready open child)
- if epic child tasks are all closed, closes the epic and exits
- runs `bd update <issue> --status in_progress`
- removes `yoke:in_review` label if present
- checks out/creates `yoke/<issue>` branch

### Implement

Do your coding work and commit changes.

### Submit for review

```bash
./bin/yoke submit bd-a1b2 \
  --done "Implemented OAuth callback" \
  --remaining "Add refresh token tests" \
  --decision "Used state nonce in callback" \
  --uncertain "Need policy on token rotation"
```

Behavior:
- runs quality checks (`.yoke/checks.sh` by default)
- adds a structured handoff note with `bd comments add`
- moves issue to review queue with `bd update <issue> --status blocked --add-label yoke:in_review`
- pushes branch to `origin` (unless `--no-push`)
- creates draft PR via `gh` (unless `--no-pr`)
- posts writer handoff comment to PR (unless `--no-pr-comment`)

### Review

```bash
./bin/yoke review bd-a1b2 --approve
# or
./bin/yoke review bd-a1b2 --reject "Missing timeout handling tests"
```

When approval succeeds and the issue PR is draft, `yoke review --approve` marks it ready for review automatically.

Optional automation hook:

```bash
./bin/yoke review bd-a1b2 --agent --approve
```

`--agent` executes `YOKE_REVIEW_CMD` with:
- `ISSUE_ID`
- `ROOT_DIR`
- `BD_PREFIX`
- `YOKE_ROLE=reviewer`

By default, reviewer actions (`--approve`, `--reject`, `--note`) also post reviewer comments to the branch PR.

## 6. Use command help aggressively

```bash
./bin/yoke --help
./bin/yoke help submit
./bin/yoke review --help
```

The help text is intentionally detailed for LLM agent consumption.
