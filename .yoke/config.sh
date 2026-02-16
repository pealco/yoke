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
YOKE_WRITER_CMD=""

# Selected coding agent for reviewing (codex or claude).
YOKE_REVIEWER_AGENT="codex"

# Optional reviewer agent command. Runs when using: yoke review --agent
# and yoke daemon. Runs with ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=reviewer.
# Expected behavior for daemon mode: execute yoke review --approve or --reject.
# Example:
# YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
YOKE_REVIEW_CMD=""

# Pull request template path.
YOKE_PR_TEMPLATE=".github/pull_request_template.md"
