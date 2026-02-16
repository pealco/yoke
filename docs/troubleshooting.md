# Troubleshooting

## `yoke: run inside a git repository`

Cause:
- command executed outside a git worktree

Fix:

```bash
cd /path/to/repo
./bin/yoke doctor
```

## `missing required command: td`

Cause:
- `td` is not installed or not on PATH

Fix:
- install `td`
- ensure shell PATH includes the install location
- rerun `yoke doctor`

## `no issue provided and td next returned nothing`

Cause:
- `yoke claim` was run with no explicit issue and `td next` had no parseable `td-...` id

Fix:
- pass explicit issue id:

```bash
yoke claim td-a1b2
```

- or ensure `td next` output includes a valid issue id
- or ensure `YOKE_TD_PREFIX` matches your td issue prefix

## `could not infer issue id from branch; pass <prefix>-xxxx explicitly`

Cause:
- `yoke submit` called without issue argument while current branch name does not contain `<YOKE_TD_PREFIX>-...`

Fix:

```bash
yoke submit td-a1b2 --done "..." --remaining "..."
```

Also verify your configured prefix:

```bash
grep YOKE_TD_PREFIX .yoke/config.sh
```

## `YOKE_REVIEW_CMD is empty in .yoke/config.sh`

Cause:
- `yoke review --agent` was used but no review command configured

Fix:
- set `YOKE_REVIEW_CMD` in `.yoke/config.sh`

Example:

```bash
YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
```

## `YOKE_WRITER_CMD is empty in .yoke/config.sh`

Cause:
- `yoke daemon` was used but no writer command is configured

Fix:
- set `YOKE_WRITER_CMD` in `.yoke/config.sh`

Example:

```bash
YOKE_WRITER_CMD='codex exec "Implement $ISSUE_ID, commit, then run yoke submit $ISSUE_ID --done \"...\" --remaining \"...\""'
```

## `writer/reviewer command did not advance issue ...`

Cause:
- daemon ran role command but td status did not change
- this indicates the role command did not call the expected lifecycle transition

Fix:
- for writer command: ensure it executes `yoke submit <issue> --done ... --remaining ...`
- for reviewer command: ensure it executes `yoke review <issue> --approve` or `--reject "..."`
- rerun with one-shot mode while debugging:

```bash
yoke daemon --once
```

## PR not created on submit

Possible causes:
- `gh` not installed
- no `origin` git remote
- open PR already exists for current branch
- `--no-pr` flag used

Diagnosis:
- inspect `yoke submit` output for skip reason

Fixes:

```bash
# verify gh
gh --version

# verify remote
git remote -v

# list open PR for current branch
gh pr list --head "$(git rev-parse --abbrev-ref HEAD)" --state open
```

## PR comment not posted on submit/review

Possible causes:
- `--no-pr-comment` flag was used
- no open PR exists for current branch
- `gh` is missing or not authenticated

Diagnosis:

```bash
gh auth status
gh pr list --head "$(git rev-parse --abbrev-ref HEAD)" --state open
```

Fixes:
- rerun without `--no-pr-comment`
- ensure branch has an open PR
- authenticate `gh`

## PR stayed draft after `yoke review --approve`

Possible causes:
- no open PR exists for issue branch
- PR is in another repository context
- `gh` lacks permission to change PR draft state

Diagnosis:

```bash
gh pr list --head "yoke/<issue-id>" --state open --json number,isDraft,url
gh auth status
```

Fixes:
- ensure the issue branch has an open PR in the current repo
- authenticate with an account that can update PR state
- rerun `yoke review <issue-id> --approve`

## `doctor failed` but command printed useful diagnostics

`yoke doctor` intentionally returns non-zero when required checks fail.

Automation pattern:

```bash
yoke doctor || true
```

Then inspect output and remediate.

## Check command failures during submit

Cause:
- `YOKE_CHECK_CMD` or `--checks` command returned non-zero

Fix:
- run the check command directly
- resolve failures
- rerun `yoke submit`

## Incorrect agent availability status

`yoke doctor` reports availability by checking known binaries on PATH.

Known mappings:
- `codex` -> `codex`
- `claude` -> `claude` or `claude-code`

If status is `not detected`:
- confirm binary name
- confirm PATH in current shell
- rerun `yoke init --no-prompt --writer-agent ... --reviewer-agent ...`
