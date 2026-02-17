# Installing yoke

This guide covers installing `yoke` on your machine.

## Requirements

- Go (1.24+ recommended)
- Git
- `bd` (required at runtime)
- `gh` (optional, needed for PR automation)

## 1. Clone the repository

```bash
git clone git@github.com:pealco/yoke.git
cd yoke
```

## 2. Install `yoke`

### Option A: Install globally (recommended)

From the repo root:

```bash
make install
```

This runs `go install ./cmd/yoke`.

### Option B: Build a local binary only

```bash
make build
```

This creates `dist/yoke`.

## 3. Ensure your Go bin directory is on `PATH`

If `yoke` is not found after `make install`, add your Go bin directory:

```bash
# zsh
export PATH="$(go env GOPATH)/bin:$PATH"
```

If you use `GOBIN`, add that directory instead.

## 4. Verify installation

```bash
yoke --help
yoke version || true
```

Then, inside a project repo using yoke:

```bash
yoke init --no-prompt --writer-agent codex --reviewer-agent codex --bd-prefix bd
yoke doctor
```

## 5. Upgrade

```bash
cd /path/to/yoke
git pull --rebase
make install
```

## 6. Uninstall

Remove the installed binary:

```bash
rm -f "$(go env GOPATH)/bin/yoke"
```

If installed via `GOBIN`, remove `$GOBIN/yoke` instead.

## Troubleshooting

- `yoke: command not found`
  - Ensure your Go bin directory is on `PATH`.
- `missing required command: bd`
  - Install `bd` and rerun `yoke doctor`.
- Need more help
  - See `docs/troubleshooting.md`.
