# yoke

`yoke` is an agent-first CLI harness for coding workflows where one agent writes and another reviews.

It uses:
- [`bd`](https://github.com/steveyegge/beads/tree/v0.49.2) as task-state control plane
- `git` branches as task workspaces
- pull requests as review/merge boundaries

## What yoke Does

- Standardizes writer/reviewer handoffs around `bd` state transitions.
- Applies quality gates before review handoff.
- Creates and updates PR flow with predictable branch naming (`yoke/<bd-id>`).
- Posts writer/reviewer handoff conversation into PR comments by default.
- Starts PRs as drafts and automatically marks them ready after final approval.
- Exposes a deterministic `yoke status` snapshot for agent preflight checks.
- Supports an automatic daemon loop for `code -> review -> code` execution.
- Provides explicit, agent-oriented `--help` output on every command.

## Documentation Map

- `/Users/pealco/archive/yoke/docs/quickstart.md`: fast setup and first end-to-end run
- `/Users/pealco/archive/yoke/docs/command-reference.md`: command-by-command reference
- `/Users/pealco/archive/yoke/docs/configuration.md`: config keys, defaults, environment overrides
- `/Users/pealco/archive/yoke/docs/how-it-works.md`: architecture and execution flow internals
- `/Users/pealco/archive/yoke/docs/agent-runbooks.md`: deterministic writer/reviewer runbooks for LLM agents
- `/Users/pealco/archive/yoke/docs/troubleshooting.md`: common failures and recovery paths
- `/Users/pealco/archive/yoke/docs/harness-v0.md`: original v0 design doc

## Install

Requirements:
- `go`
- `git`
- [`bd`](https://github.com/steveyegge/beads/tree/v0.49.2)
- `gh` (optional but recommended for PR automation)

Build and install:

```bash
# Build local binary to dist/
make build

# Install globally (Go bin path)
make install

# Build release binaries (darwin/linux, amd64/arm64)
make release
```

## Quick Usage

```bash
# 1) initialize and choose writer/reviewer agents (codex/claude)
./bin/yoke init

# 2) validate environment
./bin/yoke doctor

# 3) inspect current workflow context
./bin/yoke status

# 4) run automated loop once (or omit --once for continuous mode)
./bin/yoke daemon --once

# 5) claim work manually (optional)
./bin/yoke claim

# 6) submit work for review
./bin/yoke submit bd-a1b2 --done "Implemented X" --remaining "Add tests"

# 7) approve or reject review
./bin/yoke review bd-a1b2 --approve
# or
./bin/yoke review bd-a1b2 --reject "Missing regression test"
```

`init` also asks for the `bd` issue prefix (default: `bd`).
This prefix is used when parsing issue IDs from `bd` output and branch names.

For daemon mode, configure:
- `YOKE_WRITER_CMD` to implement and submit an issue.
- `YOKE_REVIEW_CMD` to approve/reject an issue.

## Quality

`yoke submit` runs `.yoke/checks.sh` by default.
In this repository, `.yoke/checks.sh` runs `make check`.

Primary quality entry points:

```bash
make fmt
make lint
make test
make check
```

## Agent-Friendly Help

Every command includes detailed operational help:

```bash
./bin/yoke --help
./bin/yoke help submit
./bin/yoke init --help
./bin/yoke review --help
```
