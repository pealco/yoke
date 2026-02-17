# Configuration

`yoke` reads configuration from `.yoke/config.sh`.

Default location:
- `<repo>/.yoke/config.sh`

Override location:
- `YOKE_CONFIG=/absolute/or/relative/path`

## Current config file

```bash
# shellcheck shell=bash
YOKE_BASE_BRANCH="main"
YOKE_CHECK_CMD=".yoke/checks.sh"
YOKE_BD_PREFIX="bd"
YOKE_WRITER_AGENT="codex"
YOKE_WRITER_CMD=""
YOKE_REVIEWER_AGENT="codex"
YOKE_REVIEW_CMD=""
YOKE_PR_TEMPLATE=".github/pull_request_template.md"
```

## Key reference

### `YOKE_BASE_BRANCH`

- Used by PR creation in `yoke submit`.
- Passed to `gh pr create --base`.
- Default: `main`.

### `YOKE_CHECK_CMD`

- Controls quality checks in `yoke submit`.
- Accepted forms:
  - executable path (relative or absolute)
  - shell command string
  - literal `skip` to bypass checks
- Default: `.yoke/checks.sh`.

### `YOKE_BD_PREFIX`

- Prefix used to parse bd issue IDs in command output and branch names.
- Expected issue format: `<prefix>-<id>` (example: `bd-a1b2`).
- Set during `yoke init`.
- Default: `bd`.

### `YOKE_WRITER_AGENT`

- Preferred writer agent identity (`codex` or `claude`).
- Set by `yoke init` autodetect/prompt flow.
- Current behavior: metadata/config signal for operator workflows and future routing.

### `YOKE_WRITER_CMD`

- Command executed by `yoke daemon` when processing writer work.
- Executed with `bash -lc`.
- Environment passed:
  - `ISSUE_ID`
  - `ROOT_DIR`
  - `BD_PREFIX`
  - `YOKE_ROLE=writer`
- Expected behavior: implement and transition issue to review (for example via `yoke submit`).
- Empty by default.

### `YOKE_REVIEWER_AGENT`

- Preferred reviewer agent identity (`codex` or `claude`).
- Set by `yoke init` autodetect/prompt flow.
- Current behavior: metadata/config signal for operator workflows and future routing.

### `YOKE_REVIEW_CMD`

- Command executed when `yoke review --agent` is used and by `yoke daemon` reviewer steps.
- Executed with `bash -lc`.
- Environment passed:
  - `ISSUE_ID`
  - `ROOT_DIR`
  - `BD_PREFIX`
  - `YOKE_ROLE=reviewer`
- Expected behavior for daemon mode: transition review state via `yoke review --approve` or `yoke review --reject`.
- Empty by default.

Example:

```bash
YOKE_WRITER_CMD='codex exec "Implement $ISSUE_ID, commit, then run yoke submit $ISSUE_ID --done \"...\" --remaining \"...\""'
YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
```

### `YOKE_PR_TEMPLATE`

- File used for PR body in `gh pr create --body-file`.
- Default: `.github/pull_request_template.md`.

## Related files

- `.yoke/checks.sh`: default check entrypoint invoked by `YOKE_CHECK_CMD`
- `.yoke/prompts/writer.md`: prompt scaffold for writer agents
- `.yoke/prompts/reviewer.md`: prompt scaffold for reviewer agents

## Best practices

- Keep `YOKE_CHECK_CMD` deterministic and non-interactive.
- Keep `YOKE_WRITER_CMD` and `YOKE_REVIEW_CMD` idempotent and fail-fast.
- Version-control `.yoke/config.sh` defaults appropriate for your team.
- Use `yoke doctor` after config edits.
