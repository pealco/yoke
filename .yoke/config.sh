# shellcheck shell=bash

# Base branch for PRs created by yoke.
YOKE_BASE_BRANCH="main"

# Check command or executable path. Set to "skip" to bypass.
YOKE_CHECK_CMD=".yoke/checks.sh"

# Prefix used for td issue IDs (example: td-a1b2).
YOKE_TD_PREFIX="td"

# Selected coding agent for writing (codex or claude).
YOKE_WRITER_AGENT="codex"

# Selected coding agent for reviewing (codex or claude).
YOKE_REVIEWER_AGENT="codex"

# Optional reviewer agent command. Runs when using: yoke review --agent
# ISSUE_ID and ROOT_DIR are exported for the command.
# Example:
# YOKE_REVIEW_CMD='codex run --prompt-file .yoke/prompts/reviewer.md --var issue="$ISSUE_ID"'
YOKE_REVIEW_CMD=""

# Pull request template path.
YOKE_PR_TEMPLATE=".github/pull_request_template.md"
