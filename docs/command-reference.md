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
yoke init [--writer-agent codex|claude] [--reviewer-agent codex|claude] [--bd-prefix PREFIX] [--no-prompt]
```

Purpose:
- initialize scaffold directories
- detect available coding agents
- persist writer/reviewer preferences in config

Key behavior:
- detects `codex` and `claude`/`claude-code` on PATH
- asks for bd issue prefix used to parse issue IDs (default: `bd`)
- prompts interactively when terminal is interactive and prompts are enabled
- allows same agent for writer and reviewer
- writes `.yoke/config.sh`

Failure cases:
- unknown flags
- invalid agent values
- invalid bd prefix value
- not inside a git repository

Examples:

```bash
yoke init
yoke init --writer-agent codex --reviewer-agent claude
yoke init --no-prompt --writer-agent codex --reviewer-agent codex
yoke init --no-prompt --bd-prefix bd --writer-agent codex --reviewer-agent claude
```

## `yoke doctor`

Usage:

```bash
yoke doctor
```

Purpose:
- environment and configuration validation

Checks:
- required: `git`, `bd`
- optional: `gh`
- config file presence
- configured bd prefix
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
- configured bd prefix
- configured writer/reviewer agents and availability state
- configured writer/reviewer command readiness
- bd focused issue (from current branch or latest `yoke claim` handoff when status is `in_progress` or `in_review`)
- next issue from bd (first `open` + `ready` issue)
- basic tool availability (`git`, `bd`, `gh`)

Notes:
- when `bd` is unavailable, `bd_focus` and `bd_next` are reported as `unavailable`
- when no issue is found, `bd_focus` and `bd_next` are `none`

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
- run an automatic writer/reviewer loop against `bd` issue states

Loop priority:
1. run reviewer command for focused in-review issue (from branch or latest claim), else first issue in review queue (`blocked` + label `yoke:in_review`)
2. otherwise run writer command for focused in-progress issue (from branch or latest claim)
3. otherwise claim next issue from `bd list --status open --ready`
4. otherwise idle
5. if max iterations are reached without consensus, notify and keep PR draft/open

Required config:
- `YOKE_WRITER_CMD` (unless `--writer-cmd` provided)
- `YOKE_REVIEW_CMD` (unless `--reviewer-cmd` provided)

Execution contract:
- both commands execute via `bash -lc`
- both receive env vars:
  - `ISSUE_ID`
  - `ROOT_DIR`
  - `YOKE_MAIN_ROOT`
  - `BD_PREFIX`
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
yoke claim [<prefix>-issue-id] [options]
```

Purpose:
- claim and activate a task for implementation

Options:
- `--improvement-passes <N>`: limit epic improvement passes (0-5, default: 5; `0` skips passes)

Behavior:
1. chooses issue:
   - explicit argument, or
   - first issue from `bd list --status open --ready`
2. if selected issue is an epic:
   - if `--improvement-passes 0`, skips epic improvement passes and proceeds directly to child-task selection
   - if `--improvement-passes` is greater than 0:
     - scans descendant tasks titled `Clarification needed: ...` and loads their comments as clarification context
   - if improvement is already marked complete but clarification comments exist, automatically reruns improvement
   - runs an epic improvement cycle (writer/reviewer alternating) using the configured agents
   - pass count defaults to 5 and can be limited with `--improvement-passes`
   - auto-closes clarification tasks that have comments (`bd close --reason clarified-by-comment`)
   - skips any in-progress or ready child task that still has unmet `blocks` dependencies
   - writes pass reports and summary to `.yoke/epic-improvement-reports/<epic-id>/`
   - posts an agent-generated summary comment to the epic
   - traverses epic descendants
   - prefers an `in_progress` child task if present
   - otherwise picks first ready open child task
   - if all child tasks are closed, closes the epic and exits
3. `bd update <resolved-issue> --status in_progress --remove-label yoke:in_review`
4. persist daemon focus to `<repo>/.yoke/daemon-focus` so active daemons resume this issue
5. ensure worktree `.yoke/worktrees/<resolved-issue>` exists and is attached to branch `yoke/<resolved-issue>`

Failure cases:
- `bd` missing
- no claimable issue when omitted
- explicit epic has no claimable child task (all remaining children blocked or already claimed)
- claiming an epic without available configured writer/reviewer agents
- git worktree creation or branch checkout inside worktree failure

Examples:

```bash
yoke claim
yoke claim bd-a1b2
yoke claim bd-a1b2 --improvement-passes 2
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
   - infer from current branch name containing `<YOKE_BD_PREFIX>-...`
2. run checks:
   - default from `YOKE_CHECK_CMD`
   - override with `--checks`
3. add handoff note via `bd comments add`
4. move issue to review queue via `bd update <issue> --status blocked --add-label yoke:in_review`
5. push branch to `origin` unless `--no-push`
6. open draft PR via `gh` unless `--no-pr`
   - skips PR creation when `gh` missing
   - skips PR creation when `origin` missing
   - skips PR creation when open PR already exists for branch
7. post writer handoff comment to the branch PR unless `--no-pr-comment`

Examples:

```bash
yoke submit bd-a1b2 --done "Implemented parser" --remaining "Add tests"
yoke submit --done "Refactor complete" --remaining "None" --no-pr
yoke submit bd-a1b2 --done "Done" --remaining "None" --checks "go test ./..."
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
   - first issue in review queue (`blocked` + `yoke:in_review`)
2. optional `--agent`:
   - runs shell command from `YOKE_REVIEW_CMD`
   - exports `ISSUE_ID`, `ROOT_DIR`, `BD_PREFIX`, and `YOKE_ROLE=reviewer`
3. optional `--note`:
   - `bd comments add <issue> <note>`
4. decision:
   - `--approve` -> `bd close <issue>`
   - `--reject` -> add rejection note and run `bd update <issue> --status in_progress --remove-label yoke:in_review`
   - no decision -> `bd show <issue>` and next-step hints
5. `--approve` also marks the issue PR ready for review (draft -> ready) when an open draft PR exists
6. for approve/reject/note actions, posts reviewer update comment to PR unless `--no-pr-comment`

Failure cases:
- `bd` missing
- no reviewable issue found
- `--agent` used with empty `YOKE_REVIEW_CMD`

Examples:

```bash
yoke review bd-a1b2 --approve
yoke review bd-a1b2 --reject "Missing rollback coverage"
yoke review bd-a1b2 --agent --note "Ran replay tests" --approve
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
