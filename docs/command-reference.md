# Command Reference

This reference mirrors live CLI behavior.

For current generated help text, run:

```bash
yoke --help
yoke help <command>
```

## Top-level commands

- `yoke init`
- `yoke doctor`
- `yoke status`
- `yoke daemon`
- `yoke claim`
- `yoke submit`
- `yoke review`
- `yoke help`

## `yoke init`

Usage:

```bash
yoke init [--writer-agent codex|claude] [--reviewer-agent codex|claude] [--td-prefix PREFIX] [--no-prompt]
```

Purpose:
- initialize scaffold directories
- detect available coding agents
- persist writer/reviewer preferences in config

Key behavior:
- detects `codex` and `claude`/`claude-code` on PATH
- asks for td issue prefix used to parse issue IDs (default: `td`)
- prompts interactively when terminal is interactive and prompts are enabled
- allows same agent for writer and reviewer
- writes `.yoke/config.sh`

Failure cases:
- unknown flags
- invalid agent values
- invalid td prefix value
- not inside a git repository

Examples:

```bash
yoke init
yoke init --writer-agent codex --reviewer-agent claude
yoke init --no-prompt --writer-agent codex --reviewer-agent codex
yoke init --no-prompt --td-prefix td --writer-agent codex --reviewer-agent claude
```

## `yoke doctor`

Usage:

```bash
yoke doctor
```

Purpose:
- environment and configuration validation

Checks:
- required: `git`, `td`
- optional: `gh`
- config file presence
- configured td prefix
- writer/reviewer agent availability status
- writer/reviewer daemon command status

Exit codes:
- `0` on success
- `1` if required checks fail

Example:

```bash
yoke doctor
```

## `yoke status`

Usage:

```bash
yoke status
```

Purpose:
- print deterministic repository/task/agent context before taking workflow actions

Output includes:
- repository root path
- current git branch
- configured td prefix
- configured writer/reviewer agents and availability state
- configured writer/reviewer command readiness
- td focused issue (`td current`)
- td next issue (`td next`)
- basic tool availability (`git`, `td`, `gh`)

Notes:
- when `td` is unavailable, `td_focus` and `td_next` are reported as `unavailable`
- when no issue is found, `td_focus` and `td_next` are `none`

Example:

```bash
yoke status
```

## `yoke daemon`

Usage:

```bash
yoke daemon [--once] [--interval VALUE] [--max-iterations N] [--writer-cmd CMD] [--reviewer-cmd CMD]
```

Purpose:
- run an automatic writer/reviewer loop against `td` issue states

Loop priority:
1. run reviewer command for first `td reviewable` issue
2. otherwise run writer command for focused/in-progress issue
3. otherwise claim next issue from `td next`
4. otherwise idle

Required config:
- `YOKE_WRITER_CMD` (unless `--writer-cmd` provided)
- `YOKE_REVIEW_CMD` (unless `--reviewer-cmd` provided)

Execution contract:
- both commands execute via `bash -lc`
- both receive env vars:
  - `ISSUE_ID`
  - `ROOT_DIR`
  - `TD_PREFIX`
  - `YOKE_ROLE`
- command must advance issue status; if status is unchanged, daemon exits with an error to prevent infinite loops

Examples:

```bash
yoke daemon --once
yoke daemon --interval 30s
yoke daemon --max-iterations 20
yoke daemon --writer-cmd 'echo custom writer' --reviewer-cmd 'echo custom reviewer'
```

## `yoke claim`

Usage:

```bash
yoke claim [<prefix>-issue-id]
```

Purpose:
- claim and activate a task for implementation

Behavior:
1. `td usage --new-session` (best effort)
2. chooses issue:
   - explicit argument, or
   - first issue parsed from `td next`
3. `td start <issue>`
4. switch to branch `yoke/<issue>` or create it

Failure cases:
- `td` missing
- no claimable issue when omitted
- git branch switch failure

Examples:

```bash
yoke claim
yoke claim td-a1b2
```

## `yoke submit`

Usage:

```bash
yoke submit [<prefix>-issue-id] --done "..." --remaining "..." [options]
```

Required flags:
- `--done`
- `--remaining`

Options:
- `--decision`
- `--uncertain`
- `--checks`
- `--no-push`
- `--no-pr`
- `--no-pr-comment`

Purpose:
- hand off writer output for review while enforcing checks and state transitions

Behavior:
1. resolve issue id:
   - explicit argument, or
   - infer from current branch name containing `<YOKE_TD_PREFIX>-...`
2. run checks:
   - default from `YOKE_CHECK_CMD`
   - override with `--checks`
3. run `td handoff` with provided fields
4. run `td review <issue>`
5. push branch to `origin` unless `--no-push`
6. open draft PR via `gh` unless `--no-pr`
   - skips PR creation when `gh` missing
   - skips PR creation when `origin` missing
   - skips PR creation when open PR already exists for branch
7. post writer handoff comment to the branch PR unless `--no-pr-comment`

Examples:

```bash
yoke submit td-a1b2 --done "Implemented parser" --remaining "Add tests"
yoke submit --done "Refactor complete" --remaining "None" --no-pr
yoke submit td-a1b2 --done "Done" --remaining "None" --checks "go test ./..."
```

## `yoke review`

Usage:

```bash
yoke review [<prefix>-issue-id] [--agent] [--note "..."] [--approve | --reject "..."] [--no-pr-comment]
```

Purpose:
- perform reviewer decision and advance/revert task lifecycle

Behavior:
1. select issue:
   - explicit argument, or
   - first issue parsed from `td reviewable`
2. optional `--agent`:
   - runs shell command from `YOKE_REVIEW_CMD`
   - exports `ISSUE_ID`, `ROOT_DIR`, `TD_PREFIX`, and `YOKE_ROLE=reviewer`
3. optional `--note`:
   - `td comment <issue> <note>`
4. decision:
   - `--approve` -> `td approve <issue>`
   - `--reject` -> `td reject <issue> --reason ...`
   - no decision -> `td show <issue>` and next-step hints
5. for approve/reject/note actions, posts reviewer update comment to PR unless `--no-pr-comment`

Failure cases:
- `td` missing
- no reviewable issue found
- `--agent` used with empty `YOKE_REVIEW_CMD`

Examples:

```bash
yoke review td-a1b2 --approve
yoke review td-a1b2 --reject "Missing rollback coverage"
yoke review td-a1b2 --agent --note "Ran replay tests" --approve
yoke review --note "Looks good, pending final test"
```

## `yoke help`

Usage:

```bash
yoke help [command]
```

Purpose:
- deterministic access to subcommand help text

Examples:

```bash
yoke help
yoke help submit
yoke help review
```
