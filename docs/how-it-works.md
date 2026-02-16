# How yoke Works

This document explains yoke’s execution model and internal command flow.

## Design goals

- agent-first command UX
- explicit task-state transitions
- minimal automation with strong boundaries
- deterministic behavior over implicit magic

## System model

`yoke` coordinates five systems:

1. `td` for task lifecycle
2. `git` for branch isolation
3. `gh` for PR boundary (optional)
4. local check command for quality gate
5. optional writer/reviewer shell commands for daemon automation

## Lifecycle mapping

High-level state progression:

```text
open -> in_progress -> in_review -> approved/rejected -> closed/rework
```

Command mapping:

- `yoke status` -> read-only snapshot (`git rev-parse`, `td current`, `td next`)
- `yoke daemon` -> loop (`td reviewable` -> `td in_progress` -> `td next`)
- `yoke claim` -> `td start`
- `yoke submit` -> `td handoff` + `td review`
- `yoke review --approve` -> `td approve`
- `yoke review --reject` -> `td reject`

## Execution flow diagram

```mermaid
flowchart TD
  A["yoke claim"] --> B["td start <issue>"]
  B --> C["git switch/create yoke/<issue>"]
  C --> D["implement + commit"]
  D --> E["yoke submit"]
  E --> F["run checks"]
  F --> G["td handoff"]
  G --> H["td review"]
  H --> I["push branch"]
  I --> J["create draft PR (optional)"]
  J --> K["yoke review"]
  K --> L{"decision"}
  L -->|"approve"| M["td approve"]
  L -->|"reject"| N["td reject"]
```

## Command internals

### `init`

- Ensures scaffold directories exist.
- Prompts for a td issue prefix (`YOKE_TD_PREFIX`).
- Detects available agents by searching PATH for:
  - `codex`
  - `claude` or `claude-code`
- In interactive mode, prompts for writer/reviewer choices.
- Writes normalized config values to `.yoke/config.sh`.

### `doctor`

- Validates required/optional tool availability.
- Reports configured writer/reviewer agent availability.
- Returns non-zero when required dependencies fail.

### `status`

- Prints parseable key/value snapshot fields for agents.
- Includes current branch, td focus/next, configured agents, and tool availability.
- Is read-only and safe to run before any lifecycle command.

### `daemon`

- Executes an automatic loop with action priority:
  1. review first (`td reviewable`)
  2. write next (focused or `in_progress`)
  3. claim next open issue (`td next`)
  4. idle when no actionable issues remain
- If `--max-iterations` is hit while work is still `in_progress` or `in_review`, emits a no-consensus notification and keeps PR draft/open.
- Runs `YOKE_WRITER_CMD` and `YOKE_REVIEW_CMD` with issue context env vars.
- Verifies each role command advances td state to prevent no-op infinite loops.

### `claim`

- Creates fresh td session context (`td usage --new-session`, best effort).
- Resolves target issue (explicit or `td next`) using `YOKE_TD_PREFIX`.
- Calls `td start`.
- Moves repository to task branch `yoke/<td-id>`.

### `submit`

- Resolves issue ID from argument or current branch using `YOKE_TD_PREFIX`.
- Runs configurable quality checks.
- Creates `td handoff` payload.
- Marks task as review-ready (`td review`).
- Optionally pushes and opens/updates PR path.
- Posts a writer handoff comment to the branch PR by default.

### `review`

- Resolves issue ID from argument or `td reviewable` using `YOKE_TD_PREFIX`.
- Optionally runs reviewer command hook (`YOKE_REVIEW_CMD`).
- Optionally attaches note (`td comment`).
- Applies final decision (`td approve` or `td reject`).
- Posts reviewer updates to the branch PR for approve/reject/note actions by default.
- On approve, automatically marks the issue PR ready for review when it is currently draft.

## PR behavior

`yoke submit` creates PRs only when:
- `gh` exists
- `origin` remote exists
- no open PR already exists for current branch

When an open PR exists for the current branch:
- `yoke submit` posts writer handoff summaries as PR comments.
- `yoke review` posts reviewer decisions and notes as PR comments.

Otherwise it logs a skip reason and continues.

## Error model

`yoke` exits early on first hard failure.
Typical hard failures:
- missing required tools (`td`, `git`)
- missing required flags (`submit --done`, `submit --remaining`)
- unresolved issue id
- failing check command

Typical soft skips:
- missing `gh`
- missing `origin`
- existing open PR

## Agent-oriented interface guarantees

- Every command has explicit `--help` output.
- Help text includes purpose, side effects, options, and examples.
- `yoke help <command>` offers deterministic help retrieval.

This is intentional to maximize reliability for LLM-driven execution.
