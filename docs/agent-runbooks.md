# Agent Runbooks

These runbooks are optimized for LLM coding agents that execute shell commands.

## Baseline dogfooding pattern (develop yoke with yoke)

Use this loop for every change to yoke itself.

1. Run preflight:
   - `yoke doctor`
   - `yoke status`
2. Claim work:
   - `yoke claim` or `yoke claim <issue-id>`
3. Implement and commit.
4. Submit through yoke:
   - `yoke submit <issue-id> --done "..." --remaining "..."`
5. Review through yoke:
   - `yoke review <issue-id> --approve`
   - or `yoke review <issue-id> --reject "<reason>"`
6. Keep the queue warm:
   - create at least one follow-up issue for the next iteration.

Notes:
- `<issue-id>` should match your configured prefix in `YOKE_TD_PREFIX` (default `td`).
- Prefer yoke commands over direct td/git operations for core lifecycle transitions.

## Writer agent runbook

Objective:
- implement a `td` issue and hand off for review

### Preflight

```bash
yoke doctor
```

If `doctor` fails because `td` is missing, stop and request environment setup.

### Claim issue

```bash
yoke claim
# or
yoke claim td-a1b2
```

### Implement

- modify code
- run local checks as needed
- commit changes

### Submit

```bash
yoke submit td-a1b2 \
  --done "<what is complete>" \
  --remaining "<what remains>" \
  --decision "<optional key decision>" \
  --uncertain "<optional uncertainty>"
```

Guidance:
- `--done` and `--remaining` are required.
- If on branch `yoke/td-a1b2`, issue id may be omitted.
- Avoid `--no-push`/`--no-pr` unless explicitly requested.

### Writer output contract

After `submit`, report:
- issue id
- branch name
- checks status
- whether PR was created or skipped (and why)

## Reviewer agent runbook

Objective:
- evaluate submitted issue and apply explicit approve/reject decision

### Preflight

```bash
yoke doctor
```

### Select issue

```bash
yoke review
# to target explicit issue
yoke review td-a1b2
```

No decision flags prints issue details and next steps.

### Optional automated review command

```bash
yoke review td-a1b2 --agent --note "Ran automated review pass"
```

Requires `YOKE_REVIEW_CMD` configured.

### Final decision

Approve:

```bash
yoke review td-a1b2 --approve
```

Reject:

```bash
yoke review td-a1b2 --reject "<clear rejection reason>"
```

### Reviewer output contract

Always report:
- issue id
- decision (`approve` or `reject`)
- short rationale
- any required follow-up work

## Suggested handoff templates

### Writer -> Reviewer

```text
Issue: td-a1b2
Done: <summary>
Remaining: <summary>
Decision: <optional>
Uncertain: <optional>
Checks: <passed/failed>
PR: <url or skip reason>
```

### Reviewer -> Writer (reject path)

```text
Issue: td-a1b2
Decision: reject
Reason: <specific blocking reason>
Required changes:
1. ...
2. ...
Verification required:
1. ...
2. ...
```

## Guardrails for agents

- never run destructive git commands unless explicitly requested
- prefer deterministic command output over inferred status
- include command errors verbatim when reporting failure
- call `yoke help <command>` before using unfamiliar flags
