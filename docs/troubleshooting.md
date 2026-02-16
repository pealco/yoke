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
YOKE_REVIEW_CMD='codex run --prompt-file .yoke/prompts/reviewer.md --var issue="$ISSUE_ID"'
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
