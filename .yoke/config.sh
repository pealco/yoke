# shellcheck shell=bash

# Base branch for PRs created by yoke.
YOKE_BASE_BRANCH="main"

# Check command or executable path. Set to "skip" to bypass.
YOKE_CHECK_CMD=".yoke/checks.sh"

# Prefix used for td issue IDs (example: td-a1b2).
YOKE_TD_PREFIX="td"

# Selected coding agent for writing (codex or claude).
YOKE_WRITER_AGENT="codex"

# Optional writer command for yoke daemon loops.
# Runs with ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=writer.
# Expected behavior: implement the issue and transition state via yoke submit.
YOKE_WRITER_CMD='codex exec --full-auto --cd "$ROOT_DIR" "You are the writer agent for issue $ISSUE_ID. Read .yoke/prompts/writer.md, inspect td show $ISSUE_ID, implement the issue, run checks, commit with a message referencing $ISSUE_ID, then run yoke submit $ISSUE_ID --done \"Implemented issue scope\" --remaining \"None\"."'

# Selected coding agent for reviewing (codex or claude).
YOKE_REVIEWER_AGENT="codex"

# Optional reviewer agent command. Runs when using: yoke review --agent
# and yoke daemon. Runs with ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=reviewer.
# Expected behavior for daemon mode: execute yoke review --approve or --reject.
# Example:
# YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
YOKE_REVIEW_CMD='codex exec --full-auto --cd "$ROOT_DIR" "You are the reviewer agent for issue $ISSUE_ID. Read .yoke/prompts/reviewer.md, inspect td show $ISSUE_ID and local diffs/tests, then run yoke review $ISSUE_ID --approve if there are no blocking issues, otherwise run yoke review $ISSUE_ID --reject \"Blocking issue found\"."'

# Pull request template path.
YOKE_PR_TEMPLATE=".github/pull_request_template.md"
