# yoke Documentation

This documentation is designed for both human operators and LLM coding agents.

## Read in this order

1. `/Users/pealco/archive/yoke/docs/quickstart.md`
2. `/Users/pealco/archive/yoke/docs/command-reference.md`
3. `/Users/pealco/archive/yoke/docs/configuration.md`
4. `/Users/pealco/archive/yoke/docs/how-it-works.md`
5. `/Users/pealco/archive/yoke/docs/agent-runbooks.md`
6. `/Users/pealco/archive/yoke/docs/troubleshooting.md`

## Contract

`yoke` guarantees:
- deterministic command interfaces
- explicit task state transitions
- explicit quality gates before review handoff
- optional automatic writer/reviewer loop via `yoke daemon`
- rich `--help` output for each command

`yoke` does not currently guarantee:
- automatic merge operations
- full CI/CD orchestration

It is intentionally a focused orchestration harness around `bd`, `git`, and `gh`.
