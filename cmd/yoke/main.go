// Command yoke provides a Go CLI harness for td + PR writer/reviewer workflows.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseBranch = "main"
	defaultCheckCmd   = ".yoke/checks.sh"
	defaultPRTemplate = ".github/pull_request_template.md"
	defaultTDPrefix   = "td"
	defaultDaemonPoll = 30 * time.Second
)

var (
	assignPattern = regexp.MustCompile(`^([A-Z0-9_]+)\s*=\s*(.+)$`)
	lookPath      = exec.LookPath
)

type agentSpec struct {
	ID       string
	Name     string
	Binaries []string
}

type detectedAgent struct {
	ID     string
	Name   string
	Binary string
}

var supportedAgents = []agentSpec{
	{
		ID:       "codex",
		Name:     "OpenAI Codex",
		Binaries: []string{"codex"},
	},
	{
		ID:       "claude",
		Name:     "Anthropic Claude Code",
		Binaries: []string{"claude", "claude-code"},
	},
}

type config struct {
	BaseBranch    string
	CheckCmd      string
	TDPrefix      string
	WriterAgent   string
	WriterCmd     string
	ReviewerAgent string
	ReviewCmd     string
	PRTemplate    string
	Path          string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fatal(err)
	}
}

func run(args []string) error {
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "init":
		return cmdInit(args)
	case "doctor":
		return cmdDoctor(args)
	case "status":
		return cmdStatus(args)
	case "daemon":
		return cmdDaemon(args)
	case "claim":
		return cmdClaim(args)
	case "submit":
		return cmdSubmit(args)
	case "review":
		return cmdReview(args)
	case "help", "-h", "--help":
		return cmdHelp(args)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func cmdHelp(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	if len(args) > 1 {
		return errors.New("usage: yoke help [command]")
	}

	switch args[0] {
	case "init":
		printInitUsage()
	case "doctor":
		printDoctorUsage()
	case "status":
		printStatusUsage()
	case "daemon":
		printDaemonUsage()
	case "claim":
		printClaimUsage()
	case "submit":
		printSubmitUsage()
	case "review":
		printReviewUsage()
	default:
		return fmt.Errorf("unknown help topic: %s", args[0])
	}

	return nil
}

func cmdInit(args []string) error {
	var (
		writerOverride   string
		reviewerOverride string
		tdPrefixOverride string
		noPrompt         bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--writer-agent":
			i++
			if i >= len(args) {
				return errors.New("--writer-agent requires a value")
			}
			normalized, ok := normalizeAgentID(args[i])
			if !ok {
				return fmt.Errorf("unsupported writer agent: %s", args[i])
			}
			writerOverride = normalized
		case "--reviewer-agent":
			i++
			if i >= len(args) {
				return errors.New("--reviewer-agent requires a value")
			}
			normalized, ok := normalizeAgentID(args[i])
			if !ok {
				return fmt.Errorf("unsupported reviewer agent: %s", args[i])
			}
			reviewerOverride = normalized
		case "--td-prefix":
			i++
			if i >= len(args) {
				return errors.New("--td-prefix requires a value")
			}
			normalized, err := normalizeTDPrefix(args[i])
			if err != nil {
				return err
			}
			tdPrefixOverride = normalized
		case "--no-prompt":
			noPrompt = true
		case "-h", "--help":
			printInitUsage()
			return nil
		default:
			return fmt.Errorf("unknown init argument: %s", args[i])
		}
	}

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(root, ".yoke", "prompts"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".github"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	availableAgents := detectAvailableAgents()

	tdPrefix := cfg.TDPrefix
	if tdPrefixOverride != "" {
		tdPrefix = tdPrefixOverride
	}
	if tdPrefix == "" {
		tdPrefix = defaultTDPrefix
	}

	writer := cfg.WriterAgent
	if writerOverride != "" {
		writer = writerOverride
	}

	reviewer := cfg.ReviewerAgent
	if reviewerOverride != "" {
		reviewer = reviewerOverride
	}
	if reviewer == "" && writer != "" {
		reviewer = writer
	}

	shouldPrompt := !noPrompt && isInteractiveTerminal(os.Stdin) && isInteractiveTerminal(os.Stdout)
	if shouldPrompt {
		reader := bufio.NewReader(os.Stdin)
		if tdPrefixOverride == "" {
			selected, err := promptForTDPrefix(tdPrefix, reader)
			if err != nil {
				return err
			}
			tdPrefix = selected
		}

		if len(availableAgents) > 0 {
			if writerOverride == "" {
				selected, err := promptForAgentSelection("writer", availableAgents, writer, reader)
				if err != nil {
					return err
				}
				writer = selected
			}

			if reviewerOverride == "" {
				selected, err := promptForAgentSelection("reviewer", availableAgents, reviewer, reader)
				if err != nil {
					return err
				}
				reviewer = selected
			}
		}
	}

	if writer == "" && len(availableAgents) > 0 {
		writer = availableAgents[0].ID
	}
	if reviewer == "" {
		if writer != "" {
			reviewer = writer
		} else if len(availableAgents) > 0 {
			reviewer = availableAgents[0].ID
		}
	}

	tdPrefix, err = normalizeTDPrefix(tdPrefix)
	if err != nil {
		return err
	}

	cfg.TDPrefix = tdPrefix
	cfg.WriterAgent = writer
	cfg.ReviewerAgent = reviewer
	if err := writeConfig(cfg); err != nil {
		return err
	}

	checksPath := filepath.Join(root, ".yoke", "checks.sh")
	if !fileExists(checksPath) {
		content := `#!/usr/bin/env bash
set -euo pipefail
echo "No checks configured. Edit .yoke/checks.sh."
`
		if err := os.WriteFile(checksPath, []byte(content), 0o755); err != nil {
			return err
		}
		note("Created .yoke/checks.sh")
	}

	note("Initialized yoke scaffold.")
	if len(availableAgents) == 0 {
		note("No supported coding agents detected (codex, claude). Configure manually in .yoke/config.sh.")
	}
	note("TD prefix: " + valueOrUnset(cfg.TDPrefix))
	note("Writer agent: " + valueOrUnset(cfg.WriterAgent))
	note("Reviewer agent: " + valueOrUnset(cfg.ReviewerAgent))
	note("Writer command: " + commandConfigStatus(cfg.WriterCmd))
	note("Reviewer command: " + commandConfigStatus(cfg.ReviewCmd))
	return nil
}

func cmdDoctor(args []string) error {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			printDoctorUsage()
			return nil
		}
		return fmt.Errorf("unknown doctor argument: %s", args[0])
	}

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	failures := 0
	for _, name := range []string{"git", "td"} {
		if commandExists(name) {
			note("ok: " + name)
		} else {
			note("missing: " + name)
			failures++
		}
	}

	if commandExists("gh") {
		note("ok: gh")
	} else {
		note("warning: gh missing (PR automation disabled)")
	}

	if fileExists(cfg.Path) {
		note("ok: config " + cfg.Path)
	} else {
		note("warning: config missing (" + cfg.Path + ")")
	}

	note("td prefix: " + cfg.TDPrefix)

	if cfg.WriterAgent != "" {
		note(fmt.Sprintf("writer agent: %s (%s)", cfg.WriterAgent, agentAvailabilityStatus(cfg.WriterAgent)))
	} else {
		note("writer agent: unset")
	}
	if cfg.ReviewerAgent != "" {
		note(fmt.Sprintf("reviewer agent: %s (%s)", cfg.ReviewerAgent, agentAvailabilityStatus(cfg.ReviewerAgent)))
	} else {
		note("reviewer agent: unset")
	}
	note("writer command: " + commandConfigStatus(cfg.WriterCmd))
	note("reviewer command: " + commandConfigStatus(cfg.ReviewCmd))

	if failures > 0 {
		return errors.New("doctor failed")
	}
	return nil
}

func cmdStatus(args []string) error {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			printStatusUsage()
			return nil
		}
		return fmt.Errorf("unknown status argument: %s", args[0])
	}

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	branch := strings.TrimSpace(commandCombinedOutput("git", "rev-parse", "--abbrev-ref", "HEAD"))
	tdAvailable := commandExists("td")

	tdFocus := "unavailable"
	tdNext := "unavailable"
	if tdAvailable {
		tdFocus = issueOrNone(focusedIssueID(cfg.TDPrefix))
		tdNext = issueOrNone(extractIssueID(commandCombinedOutput("td", "next"), cfg.TDPrefix))
	}

	note("repo_root: " + root)
	note("current_branch: " + valueOrFallback(branch, "unknown"))
	note("td_prefix: " + cfg.TDPrefix)
	note("writer_agent: " + valueOrUnset(cfg.WriterAgent))
	note("writer_agent_status: " + configuredAgentStatus(cfg.WriterAgent))
	note("writer_command: " + commandConfigStatus(cfg.WriterCmd))
	note("reviewer_agent: " + valueOrUnset(cfg.ReviewerAgent))
	note("reviewer_agent_status: " + configuredAgentStatus(cfg.ReviewerAgent))
	note("reviewer_command: " + commandConfigStatus(cfg.ReviewCmd))
	note("td_focus: " + tdFocus)
	note("td_next: " + tdNext)
	note("tool_git: " + availabilityLabel(commandExists("git")))
	note("tool_td: " + availabilityLabel(tdAvailable))
	note("tool_gh: " + availabilityLabel(commandExists("gh")))
	return nil
}

type daemonLoopOptions struct {
	Once          bool
	Interval      time.Duration
	MaxIterations int
	WriterCmd     string
	ReviewerCmd   string
}

func cmdDaemon(args []string) error {
	options := daemonLoopOptions{
		Interval: defaultDaemonPoll,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--once":
			options.Once = true
		case "--interval":
			i++
			if i >= len(args) {
				return errors.New("--interval requires a value")
			}
			interval, err := parseDaemonInterval(args[i])
			if err != nil {
				return err
			}
			options.Interval = interval
		case "--max-iterations":
			i++
			if i >= len(args) {
				return errors.New("--max-iterations requires a value")
			}
			parsed, err := strconv.Atoi(args[i])
			if err != nil || parsed <= 0 {
				return fmt.Errorf("invalid --max-iterations value: %s", args[i])
			}
			options.MaxIterations = parsed
		case "--writer-cmd":
			i++
			if i >= len(args) {
				return errors.New("--writer-cmd requires a value")
			}
			options.WriterCmd = args[i]
		case "--reviewer-cmd":
			i++
			if i >= len(args) {
				return errors.New("--reviewer-cmd requires a value")
			}
			options.ReviewerCmd = args[i]
		case "-h", "--help":
			printDaemonUsage()
			return nil
		default:
			return fmt.Errorf("unknown daemon argument: %s", args[i])
		}
	}

	if !commandExists("td") {
		return fmt.Errorf("missing required command: td")
	}

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	if strings.TrimSpace(options.WriterCmd) == "" {
		options.WriterCmd = cfg.WriterCmd
	}
	if strings.TrimSpace(options.ReviewerCmd) == "" {
		options.ReviewerCmd = cfg.ReviewCmd
	}
	if strings.TrimSpace(options.WriterCmd) == "" {
		return errors.New("YOKE_WRITER_CMD is empty in .yoke/config.sh (required for yoke daemon)")
	}
	if strings.TrimSpace(options.ReviewerCmd) == "" {
		return errors.New("YOKE_REVIEW_CMD is empty in .yoke/config.sh (required for yoke daemon)")
	}

	note("Daemon started.")
	note("  poll interval: " + options.Interval.String())
	if options.Once {
		note("  mode: once")
	} else {
		note("  mode: continuous")
	}
	if options.MaxIterations > 0 {
		note(fmt.Sprintf("  max iterations: %d", options.MaxIterations))
	}

	for iteration := 1; ; iteration++ {
		action, err := runDaemonIteration(root, cfg, options.WriterCmd, options.ReviewerCmd)
		if err != nil {
			return err
		}

		if options.Once {
			note("Daemon completed single iteration: " + action)
			return nil
		}
		if options.MaxIterations > 0 && iteration >= options.MaxIterations {
			note(fmt.Sprintf("Daemon reached max iterations (%d); exiting.", options.MaxIterations))
			return nil
		}

		if action == "idle" {
			time.Sleep(options.Interval)
		}
	}
}

func parseDaemonInterval(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("interval cannot be empty")
	}

	duration, durationErr := time.ParseDuration(value)
	if durationErr == nil {
		if duration <= 0 {
			return 0, fmt.Errorf("interval must be positive: %s", raw)
		}
		return duration, nil
	}

	seconds, intErr := strconv.Atoi(value)
	if intErr != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid interval %q: use positive seconds (e.g. 30) or duration (e.g. 30s, 1m)", raw)
	}
	return time.Duration(seconds) * time.Second, nil
}

func runDaemonIteration(root string, cfg config, writerCmd, reviewerCmd string) (string, error) {
	reviewable := firstReviewableIssueID(cfg.TDPrefix)
	if reviewable != "" {
		if err := runDaemonRoleCommand("reviewer", reviewable, reviewerCmd, root, cfg.TDPrefix); err != nil {
			return "", err
		}
		return "reviewed " + reviewable, nil
	}

	inProgress, err := focusedOrInProgressIssueID(cfg.TDPrefix)
	if err != nil {
		return "", err
	}
	if inProgress != "" {
		if err := ensureIssueBranchCheckedOut(inProgress); err != nil {
			return "", err
		}
		if err := runDaemonRoleCommand("writer", inProgress, writerCmd, root, cfg.TDPrefix); err != nil {
			return "", err
		}
		return "wrote " + inProgress, nil
	}

	next := nextIssueID(cfg.TDPrefix)
	if next != "" {
		note("Daemon claiming next issue: " + next)
		if err := cmdClaim([]string{next}); err != nil {
			return "", err
		}
		return "claimed " + next, nil
	}

	return "idle", nil
}

func runDaemonRoleCommand(role, issue, shellCommand, root, tdPrefix string) error {
	previousStatus, err := issueStatus(issue)
	if err != nil {
		return err
	}

	note(fmt.Sprintf("Daemon running %s command for %s", role, issue))
	cmd := exec.Command("bash", "-lc", shellCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"ISSUE_ID="+issue,
		"ROOT_DIR="+root,
		"TD_PREFIX="+tdPrefix,
		"YOKE_ROLE="+role,
	)
	if err := cmd.Run(); err != nil {
		return err
	}

	currentStatus, err := issueStatus(issue)
	if err != nil {
		return err
	}
	if currentStatus == previousStatus {
		return fmt.Errorf("%s command did not advance issue %s (still %s); ensure the command transitions td state", role, issue, currentStatus)
	}

	note(fmt.Sprintf("Daemon observed %s status transition: %s -> %s", issue, previousStatus, currentStatus))
	return nil
}

func focusedOrInProgressIssueID(prefix string) (string, error) {
	focused := focusedIssueID(prefix)
	if focused != "" {
		status, err := issueStatus(focused)
		if err != nil {
			return "", err
		}
		if status == "in_progress" {
			return focused, nil
		}
	}
	return firstIssueByStatus(prefix, "in_progress")
}

func focusedIssueID(prefix string) string {
	return parseFocusedIssueID(commandCombinedOutput("td", "current"), prefix)
}

func parseFocusedIssueID(raw, prefix string) string {
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "FOCUSED:") {
			continue
		}
		return extractIssueID(trimmed, prefix)
	}
	return ""
}

func ensureIssueBranchCheckedOut(issue string) error {
	target := branchForIssue(issue)
	current := strings.TrimSpace(commandCombinedOutput("git", "rev-parse", "--abbrev-ref", "HEAD"))
	if current == target {
		return nil
	}

	if refExists("refs/heads/" + target) {
		return runCommand("git", "switch", target)
	}
	return runCommand("git", "switch", "-c", target)
}

type tdListIssue struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func firstIssueByStatus(prefix, status string) (string, error) {
	output := commandCombinedOutput("td", "list", "--status", status, "--format", "json", "--limit", "20")
	issues, err := parseTDListIssuesJSON(output)
	if err != nil {
		return "", err
	}
	return firstMatchingIssueID(issues, prefix, status), nil
}

func parseTDListIssuesJSON(raw string) ([]tdListIssue, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var issues []tdListIssue
	if err := json.Unmarshal([]byte(trimmed), &issues); err != nil {
		return nil, fmt.Errorf("parse td list json: %w", err)
	}
	return issues, nil
}

func firstMatchingIssueID(issues []tdListIssue, prefix, status string) string {
	targetStatus := strings.ToLower(strings.TrimSpace(status))
	for _, issue := range issues {
		issueID := strings.ToLower(strings.TrimSpace(issue.ID))
		issueStatus := strings.ToLower(strings.TrimSpace(issue.Status))
		if issueID == "" {
			continue
		}
		if targetStatus != "" && issueStatus != targetStatus {
			continue
		}
		if looksLikeIssueID(issueID, prefix) {
			return issueID
		}
	}
	return ""
}

func issueStatus(issue string) (string, error) {
	output := commandCombinedOutput("td", "show", issue, "--json")
	return parseIssueStatusJSON(output)
}

func parseIssueStatusJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty issue payload")
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("parse td show json: %w", err)
	}

	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		return "", errors.New("issue payload missing status")
	}
	return status, nil
}

func cmdClaim(args []string) error {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			printClaimUsage()
			return nil
		}
	}

	if len(args) > 1 {
		return fmt.Errorf("usage: yoke claim [td-issue-id]")
	}

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if !commandExists("td") {
		return fmt.Errorf("missing required command: td")
	}

	issue := ""
	if len(args) == 1 {
		issue = args[0]
	}

	_ = runCommandDiscard("td", "usage", "--new-session")

	if issue == "" {
		issue = nextIssueID(cfg.TDPrefix)
	}
	if issue == "" {
		return errors.New("no issue provided and td next returned nothing")
	}

	if err := runCommand("td", "start", issue); err != nil {
		return err
	}

	branch := branchForIssue(issue)
	if refExists("refs/heads/" + branch) {
		if err := runCommand("git", "switch", branch); err != nil {
			return err
		}
	} else {
		if err := runCommand("git", "switch", "-c", branch); err != nil {
			return err
		}
	}

	note(fmt.Sprintf("Claimed %s on branch %s", issue, branch))
	note(fmt.Sprintf("Next: yoke submit %s --done \"...\" --remaining \"...\"", issue))
	return nil
}

func cmdSubmit(args []string) error {
	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	var (
		issue     string
		doneText  string
		remaining string
		decision  string
		uncertain string
		checks    string
		noPush    bool
		noPR      bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--done":
			i++
			if i >= len(args) {
				return errors.New("--done requires text")
			}
			doneText = args[i]
		case "--remaining":
			i++
			if i >= len(args) {
				return errors.New("--remaining requires text")
			}
			remaining = args[i]
		case "--decision":
			i++
			if i >= len(args) {
				return errors.New("--decision requires text")
			}
			decision = args[i]
		case "--uncertain":
			i++
			if i >= len(args) {
				return errors.New("--uncertain requires text")
			}
			uncertain = args[i]
		case "--checks":
			i++
			if i >= len(args) {
				return errors.New("--checks requires text")
			}
			checks = args[i]
		case "--no-push":
			noPush = true
		case "--no-pr":
			noPR = true
		case "-h", "--help":
			printSubmitUsage()
			return nil
		default:
			if looksLikeIssueID(arg, cfg.TDPrefix) {
				if issue != "" {
					return errors.New("multiple issue ids provided")
				}
				issue = arg
				continue
			}
			return fmt.Errorf("unknown submit argument: %s", arg)
		}
	}

	if !commandExists("td") {
		return fmt.Errorf("missing required command: td")
	}
	if doneText == "" {
		return errors.New("--done is required")
	}
	if remaining == "" {
		return errors.New("--remaining is required")
	}

	if issue == "" {
		issue = currentBranchIssue(cfg.TDPrefix)
	}
	if issue == "" {
		return fmt.Errorf("could not infer issue id from branch; pass %s-xxxx explicitly", cfg.TDPrefix)
	}

	checkCommand := cfg.CheckCmd
	if checks != "" {
		checkCommand = checks
	}
	if err := runChecks(root, checkCommand); err != nil {
		return err
	}

	handoffArgs := []string{"handoff", issue, "--done", doneText, "--remaining", remaining}
	if decision != "" {
		handoffArgs = append(handoffArgs, "--decision", decision)
	}
	if uncertain != "" {
		handoffArgs = append(handoffArgs, "--uncertain", uncertain)
	}
	if err := runCommand("td", handoffArgs...); err != nil {
		return err
	}

	if err := runCommand("td", "review", issue); err != nil {
		return err
	}

	if !noPush {
		if hasOriginRemote() {
			if err := runCommand("git", "push", "-u", "origin", "HEAD"); err != nil {
				return err
			}
		} else {
			note("No origin remote; skipping push.")
		}
	}

	if !noPR {
		title := issueTitle(issue)
		if err := createPRIfNeeded(root, cfg, issue, title); err != nil {
			return err
		}
	}

	note(fmt.Sprintf("Submitted %s for review.", issue))
	note(fmt.Sprintf("Reviewer: yoke review %s", issue))
	return nil
}

func cmdReview(args []string) error {
	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	var (
		issue        string
		action       string
		rejectReason string
		noteText     string
		runAgent     bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--approve":
			action = "approve"
		case "--reject":
			i++
			if i >= len(args) {
				return errors.New("--reject requires a reason")
			}
			action = "reject"
			rejectReason = args[i]
		case "--note":
			i++
			if i >= len(args) {
				return errors.New("--note requires text")
			}
			noteText = args[i]
		case "--agent":
			runAgent = true
		case "-h", "--help":
			printReviewUsage()
			return nil
		default:
			if looksLikeIssueID(arg, cfg.TDPrefix) {
				if issue != "" {
					return errors.New("multiple issue ids provided")
				}
				issue = arg
				continue
			}
			return fmt.Errorf("unknown review argument: %s", arg)
		}
	}

	if !commandExists("td") {
		return fmt.Errorf("missing required command: td")
	}

	if issue == "" {
		issue = firstReviewableIssueID(cfg.TDPrefix)
	}
	if issue == "" {
		return errors.New("no reviewable issue found")
	}

	if runAgent {
		if strings.TrimSpace(cfg.ReviewCmd) == "" {
			return errors.New("YOKE_REVIEW_CMD is empty in .yoke/config.sh")
		}
		note("Running reviewer agent for " + issue)
		cmd := exec.Command("bash", "-lc", cfg.ReviewCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			"ISSUE_ID="+issue,
			"ROOT_DIR="+root,
			"TD_PREFIX="+cfg.TDPrefix,
			"YOKE_ROLE=reviewer",
		)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	if noteText != "" {
		if err := runCommand("td", "comment", issue, noteText); err != nil {
			return err
		}
	}

	switch action {
	case "approve":
		if err := runCommand("td", "approve", issue); err != nil {
			return err
		}
		note("Approved " + issue)
	case "reject":
		if err := runCommand("td", "reject", issue, "--reason", rejectReason); err != nil {
			return err
		}
		note("Rejected " + issue)
	default:
		if err := runCommand("td", "show", issue); err != nil {
			return err
		}
		note("Next:")
		note("  yoke review " + issue + " --approve")
		note("  yoke review " + issue + " --reject \"reason\"")
	}

	return nil
}

func loadConfig(root string) (config, error) {
	path := os.Getenv("YOKE_CONFIG")
	if path == "" {
		path = filepath.Join(root, ".yoke", "config.sh")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}

	cfg := config{
		BaseBranch:    defaultBaseBranch,
		CheckCmd:      defaultCheckCmd,
		TDPrefix:      defaultTDPrefix,
		WriterAgent:   "",
		WriterCmd:     "",
		ReviewerAgent: "",
		ReviewCmd:     "",
		PRTemplate:    defaultPRTemplate,
		Path:          path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := assignPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		key := matches[1]
		value := parseShellValue(matches[2])

		switch key {
		case "YOKE_BASE_BRANCH":
			cfg.BaseBranch = value
		case "YOKE_CHECK_CMD":
			cfg.CheckCmd = value
		case "YOKE_TD_PREFIX":
			cfg.TDPrefix = value
		case "YOKE_WRITER_AGENT":
			cfg.WriterAgent = value
		case "YOKE_WRITER_CMD":
			cfg.WriterCmd = value
		case "YOKE_REVIEWER_AGENT":
			cfg.ReviewerAgent = value
		case "YOKE_REVIEW_CMD":
			cfg.ReviewCmd = value
		case "YOKE_PR_TEMPLATE":
			cfg.PRTemplate = value
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}

	normalizedPrefix, err := normalizeTDPrefix(cfg.TDPrefix)
	if err != nil {
		return cfg, err
	}
	cfg.TDPrefix = normalizedPrefix

	return cfg, nil
}

func parseShellValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) && len(value) >= 2 {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, `"`)
	}
	if strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`) && len(value) >= 2 {
		return strings.Trim(value, `'`)
	}

	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func writeConfig(cfg config) error {
	content := renderConfig(cfg)
	return os.WriteFile(cfg.Path, []byte(content), 0o644)
}

func renderConfig(cfg config) string {
	return fmt.Sprintf(`# shellcheck shell=bash

# Base branch for PRs created by yoke.
YOKE_BASE_BRANCH=%s

# Check command or executable path. Set to "skip" to bypass.
YOKE_CHECK_CMD=%s

# Prefix used for td issue IDs (example: td-a1b2).
YOKE_TD_PREFIX=%s

# Selected coding agent for writing (codex or claude).
YOKE_WRITER_AGENT=%s

# Optional writer command for yoke daemon loops.
# Runs with ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=writer.
# Expected behavior: implement the issue and transition state via yoke submit.
YOKE_WRITER_CMD=%s

# Selected coding agent for reviewing (codex or claude).
YOKE_REVIEWER_AGENT=%s

# Optional reviewer agent command. Runs when using: yoke review --agent
# and yoke daemon. Runs with ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=reviewer.
# Expected behavior for daemon mode: execute yoke review --approve or --reject.
# Example:
# YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
YOKE_REVIEW_CMD=%s

# Pull request template path.
YOKE_PR_TEMPLATE=%s
`,
		quoteShell(cfg.BaseBranch),
		quoteShell(cfg.CheckCmd),
		quoteShell(cfg.TDPrefix),
		quoteShell(cfg.WriterAgent),
		quoteShell(cfg.WriterCmd),
		quoteShell(cfg.ReviewerAgent),
		quoteShell(cfg.ReviewCmd),
		quoteShell(cfg.PRTemplate),
	)
}

func quoteShell(value string) string {
	return strconv.Quote(value)
}

func ensureRepoRoot() (string, error) {
	root, err := commandOutput("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("run inside a git repository")
	}
	return strings.TrimSpace(root), nil
}

func detectAvailableAgents() []detectedAgent {
	available := make([]detectedAgent, 0, len(supportedAgents))
	for _, spec := range supportedAgents {
		for _, binary := range spec.Binaries {
			if commandExists(binary) {
				available = append(available, detectedAgent{
					ID:     spec.ID,
					Name:   spec.Name,
					Binary: binary,
				})
				break
			}
		}
	}

	return available
}

func normalizeAgentID(input string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(input))
	for _, spec := range supportedAgents {
		if value == spec.ID {
			return spec.ID, true
		}
		for _, binary := range spec.Binaries {
			if value == strings.ToLower(binary) {
				return spec.ID, true
			}
		}
	}

	return "", false
}

func normalizeTDPrefix(input string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return defaultTDPrefix, nil
	}

	validPrefix := regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?$`)
	if !validPrefix.MatchString(value) {
		return "", fmt.Errorf("invalid td prefix %q: use letters, numbers, '_' or '-', and avoid trailing '-'", input)
	}

	return value, nil
}

func containsAgentID(agents []detectedAgent, id string) bool {
	for _, agent := range agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func promptForAgentSelection(
	role string,
	available []detectedAgent,
	current string,
	reader *bufio.Reader,
) (string, error) {
	if len(available) == 0 {
		return current, nil
	}

	defaultAgent := current
	if defaultAgent == "" || !containsAgentID(available, defaultAgent) {
		defaultAgent = available[0].ID
	}

	for {
		note(fmt.Sprintf("Select %s agent:", role))
		for i, agent := range available {
			marker := ""
			if agent.ID == defaultAgent {
				marker = " (default)"
			}
			note(fmt.Sprintf("  %d) %s [%s]%s", i+1, agent.ID, agent.Binary, marker))
		}
		fmt.Printf("%s agent [%s]: ", titleCase(role), defaultAgent)

		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return defaultAgent, nil
		}

		if index, convErr := strconv.Atoi(trimmed); convErr == nil {
			if index >= 1 && index <= len(available) {
				return available[index-1].ID, nil
			}
			note("Invalid selection. Enter a valid option number or agent name.")
			if errors.Is(err, io.EOF) {
				return defaultAgent, nil
			}
			continue
		}

		if normalized, ok := normalizeAgentID(trimmed); ok && containsAgentID(available, normalized) {
			return normalized, nil
		}

		note("Invalid selection. Enter a valid option number or agent name.")
		if errors.Is(err, io.EOF) {
			return defaultAgent, nil
		}
	}
}

func promptForTDPrefix(current string, reader *bufio.Reader) (string, error) {
	defaultPrefix, err := normalizeTDPrefix(current)
	if err != nil {
		defaultPrefix = defaultTDPrefix
	}

	for {
		fmt.Printf("TD issue prefix [%s]: ", defaultPrefix)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return defaultPrefix, nil
		}

		normalized, normErr := normalizeTDPrefix(trimmed)
		if normErr == nil {
			return normalized, nil
		}
		note(normErr.Error())

		if errors.Is(readErr, io.EOF) {
			return defaultPrefix, nil
		}
	}
}

func isInteractiveTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unset"
	}
	return value
}

func valueOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func issueOrNone(issue string) string {
	if strings.TrimSpace(issue) == "" {
		return "none"
	}
	return issue
}

func configuredAgentStatus(agentID string) string {
	if strings.TrimSpace(agentID) == "" {
		return "unset"
	}
	return agentAvailabilityStatus(agentID)
}

func commandConfigStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unset"
	}
	return "configured"
}

func availabilityLabel(available bool) string {
	if available {
		return "available"
	}
	return "missing"
}

func agentAvailabilityStatus(agentID string) string {
	normalized, ok := normalizeAgentID(agentID)
	if !ok {
		return "unknown"
	}

	for _, spec := range supportedAgents {
		if spec.ID != normalized {
			continue
		}
		for _, binary := range spec.Binaries {
			if commandExists(binary) {
				return "available via " + binary
			}
		}
		return "not detected"
	}

	return "unknown"
}

func commandExists(name string) bool {
	_, err := lookPath(name)
	return err == nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCommandDiscard(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

func commandCombinedOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func refExists(ref string) bool {
	err := runCommandDiscard("git", "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func issuePatternForPrefix(prefix string) *regexp.Regexp {
	normalized, err := normalizeTDPrefix(prefix)
	if err != nil {
		normalized = defaultTDPrefix
	}
	return regexp.MustCompile(regexp.QuoteMeta(normalized) + `-[a-z0-9]+`)
}

func extractIssueID(s, prefix string) string {
	return issuePatternForPrefix(prefix).FindString(strings.ToLower(s))
}

func looksLikeIssueID(value, prefix string) bool {
	pattern := issuePatternForPrefix(prefix)
	return pattern.FindString(strings.ToLower(value)) == strings.ToLower(value)
}

func nextIssueID(prefix string) string {
	output := commandCombinedOutput("td", "next")
	return extractIssueID(output, prefix)
}

func firstReviewableIssueID(prefix string) string {
	output := commandCombinedOutput("td", "reviewable")
	return extractIssueID(output, prefix)
}

func currentBranchIssue(prefix string) string {
	output, err := commandOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return extractIssueID(output, prefix)
}

func branchForIssue(issue string) string {
	return "yoke/" + issue
}

func issueTitle(issue string) string {
	output := commandCombinedOutput("td", "show", issue)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Title:") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			if title != "" {
				return title
			}
		}
	}
	return issue
}

func runChecks(root, checkCmd string) error {
	if checkCmd == "" {
		checkCmd = defaultCheckCmd
	}
	if checkCmd == "skip" {
		note("Skipping checks (YOKE_CHECK_CMD=skip).")
		return nil
	}

	resolved := resolveRepoPath(root, checkCmd)
	if isExecutable(resolved) {
		note("Running checks via " + resolved)
		return runCommand(resolved)
	}

	note("Running checks: " + checkCmd)
	cmd := exec.Command("bash", "-lc", checkCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = root
	return cmd.Run()
}

func resolveRepoPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func createPRIfNeeded(root string, cfg config, issue, title string) error {
	if !commandExists("gh") {
		note("gh not found; skipping PR creation.")
		return nil
	}
	if !hasOriginRemote() {
		note("No origin remote; skipping PR creation.")
		return nil
	}

	branchOutput, err := commandOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	branch := strings.TrimSpace(branchOutput)
	if branch == "" {
		return errors.New("could not determine current branch")
	}

	existing := strings.TrimSpace(commandCombinedOutput(
		"gh", "pr", "list",
		"--head", branch,
		"--state", "open",
		"--json", "number",
		"--jq", ".[0].number",
	))
	if existing != "" && existing != "null" {
		note(fmt.Sprintf("PR #%s already exists for %s.", existing, branch))
		return nil
	}

	templatePath := resolveRepoPath(root, cfg.PRTemplate)
	createArgs := []string{
		"pr", "create",
		"--draft",
		"--base", cfg.BaseBranch,
		"--title", fmt.Sprintf("[%s] %s", issue, title),
	}
	if fileExists(templatePath) {
		createArgs = append(createArgs, "--body-file", templatePath)
	} else {
		createArgs = append(createArgs, "--body", "")
	}
	return runCommand("gh", createArgs...)
}

func hasOriginRemote() bool {
	_, err := commandOutput("git", "remote", "get-url", "origin")
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func note(msg string) {
	fmt.Println(msg)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "yoke: %s\n", err)
	os.Exit(1)
}

func printUsage() {
	fmt.Print(`yoke: agent-first td + PR harness

Purpose:
  Coordinate writer/reviewer workflows for coding agents using td state transitions
  and git/PR boundaries.

Usage:
  yoke init [options]
  yoke doctor
  yoke status
  yoke daemon [options]
  yoke claim [<prefix>-issue-id]
  yoke submit [<prefix>-issue-id] --done "..." --remaining "..." [options]
  yoke review [<prefix>-issue-id] [options]
  yoke help [command]

Commands:
  init    Initialize scaffold, detect available agents, and persist writer/reviewer choices.
  doctor  Validate required tools/config and report agent availability.
  status  Print current repo/task/agent status snapshot for deterministic agent consumption.
  daemon  Run continuous writer/reviewer automation loop over td issue states.
  claim   Start work on an issue (td start + branch switch/create).
  submit  Run checks, create td handoff, move issue to review, and open/update PR workflow.
  review  Review an issue, optionally run reviewer automation, then approve/reject.

Help discovery:
  yoke <command> --help
  yoke help <command>
`)
}

func printInitUsage() {
	fmt.Print(`Usage:
  yoke init [options]

Purpose:
  Prepare yoke for use and set preferred coding agents.

Behavior:
  1) Ensures scaffold directories exist (.yoke/, .github/, docs/).
  2) Autodetects supported agents on PATH (codex, claude/claude-code).
  3) In interactive terminals, prompts for td issue prefix selection.
  4) In interactive terminals, prompts for writer and reviewer selection.
     Writer and reviewer may be the same agent.
  5) Writes selections to .yoke/config.sh.

Options:
  --writer-agent codex|claude     Set writer agent explicitly.
  --reviewer-agent codex|claude   Set reviewer agent explicitly.
  --td-prefix PREFIX              Set td issue prefix explicitly (default: td).
  --no-prompt                     Do not prompt; auto-select detected defaults.

Examples:
  yoke init
  yoke init --writer-agent codex --reviewer-agent codex
  yoke init --no-prompt --writer-agent codex --reviewer-agent claude --td-prefix td

Outputs:
  Updates .yoke/config.sh keys:
  - YOKE_TD_PREFIX
  - YOKE_WRITER_AGENT
  - YOKE_WRITER_CMD
  - YOKE_REVIEWER_AGENT
  - YOKE_REVIEW_CMD
`)
}

func printDoctorUsage() {
	fmt.Print(`Usage:
  yoke doctor

Purpose:
  Validate local environment before running writer/reviewer workflows.

Checks performed:
  - Required binaries: git, td
  - Optional binary: gh
  - Config file presence: .yoke/config.sh
  - Configured td issue prefix
  - Configured writer/reviewer agent availability on PATH
  - Configured writer/reviewer daemon commands

Exit behavior:
  - Exit 0 when required checks pass.
  - Exit 1 when any required check fails.

Example:
  yoke doctor
`)
}

func printStatusUsage() {
	fmt.Print(`Usage:
  yoke status

Purpose:
  Print a deterministic status snapshot that coding agents can parse before acting.

Output fields:
  - repo_root: git repository root path
  - current_branch: active branch name
  - td_prefix: configured issue prefix from YOKE_TD_PREFIX
  - writer_agent / reviewer_agent: configured agent ids (or unset)
  - writer_agent_status / reviewer_agent_status: binary availability summary
  - writer_command / reviewer_command: daemon command readiness
  - td_focus: focused issue parsed from td current (or none/unavailable)
  - td_next: next issue parsed from td next (or none/unavailable)
  - tool_git / tool_td / tool_gh: command availability

Usage guidance for agents:
  1) Run yoke status before claim/submit/review to confirm context.
  2) If td_focus is none, prefer yoke claim.
  3) If reviewer_agent_status is missing, use manual yoke review flags.

Example:
  yoke status
`)
}

func printDaemonUsage() {
	fmt.Print(`Usage:
  yoke daemon [options]

Purpose:
  Run an automatic code -> review loop for td issues using configured writer/reviewer commands.

Loop priority (each iteration):
  1) Review first issue from td reviewable via reviewer command.
  2) Otherwise run writer command on focused/in_progress issue.
  3) Otherwise claim td next issue.
  4) Otherwise idle (sleep and poll again in continuous mode).

Command contract:
  - Writer command comes from YOKE_WRITER_CMD (or --writer-cmd override).
  - Reviewer command comes from YOKE_REVIEW_CMD (or --reviewer-cmd override).
  - Both run with env vars:
      ISSUE_ID, ROOT_DIR, TD_PREFIX, YOKE_ROLE
  - Commands must transition td status (writer -> submit/review, reviewer -> approve/reject).
    If status does not change, daemon exits with an error to avoid infinite loops.

Options:
  --once                    Run a single iteration and exit.
  --interval VALUE          Poll interval for idle loops. Accepts seconds (30) or durations (30s, 1m).
  --max-iterations N        Stop after N iterations in continuous mode.
  --writer-cmd CMD          Override writer command for this daemon run.
  --reviewer-cmd CMD        Override reviewer command for this daemon run.

Examples:
  yoke daemon --once
  yoke daemon --interval 45s
  yoke daemon --max-iterations 10
`)
}

func printClaimUsage() {
	fmt.Print(`Usage:
  yoke claim [<prefix>-issue-id]

Purpose:
  Move an issue into active work and place the repository on the matching branch.

Behavior:
  - Starts a fresh td session context (td usage --new-session).
  - If issue id omitted, pulls from td next.
  - Runs td start <issue>.
  - Switches to existing branch yoke/<issue> or creates it.

Inputs:
  issue-id    Optional. Explicit issue id (example uses prefix from YOKE_TD_PREFIX).

Examples:
  yoke claim
  yoke claim td-a1b2

Side effects:
  - td status transition to in_progress
  - git branch switch/create
`)
}

func printSubmitUsage() {
	fmt.Print(`Usage:
  yoke submit [<prefix>-issue-id] --done "..." --remaining "..." [options]

Purpose:
  Handoff implementation from writer to reviewer with explicit task state updates.

Behavior:
  1) Runs checks (default: .yoke/checks.sh).
  2) Writes td handoff fields.
  3) Moves issue to review (td review).
  4) Pushes branch and opens draft PR when configured tools are available.

Inputs:
  issue-id    Optional. If omitted, inferred from current branch name using YOKE_TD_PREFIX.

Options:
  --done TEXT          Required. What is complete now.
  --remaining TEXT     Required. What remains.
  --decision TEXT      Optional. Key decision made.
  --uncertain TEXT     Optional. Open uncertainty.
  --checks CMD         Optional. Override check command/script.
  --no-push            Do not push branch.
  --no-pr              Do not create or update PR.

Examples:
  yoke submit td-a1b2 --done "Added auth flow" --remaining "Add tests"
  yoke submit --done "Refactor complete" --remaining "None" --no-pr
`)
}

func printReviewUsage() {
	fmt.Print(`Usage:
  yoke review [<prefix>-issue-id] [options]

Purpose:
  Execute reviewer step and finalize review outcome for a td issue.

Behavior:
  - If issue id omitted, selects first item from td reviewable.
  - Optional reviewer automation can run before final action.
  - Reviewer automation receives ISSUE_ID, ROOT_DIR, TD_PREFIX, and YOKE_ROLE=reviewer.
  - Approve closes review path; reject returns work to writer path.

Inputs:
  issue-id    Optional. Explicit issue id using YOKE_TD_PREFIX.

Options:
  --agent              Run YOKE_REVIEW_CMD before final action.
  --note TEXT          Add reviewer note to td issue.
  --approve            Approve issue (td approve).
  --reject TEXT        Reject issue with reason (td reject --reason).

Examples:
  yoke review td-a1b2 --agent --approve
  yoke review td-a1b2 --reject "Missing edge-case test coverage"
  yoke review --note "Verified behavior locally"
`)
}
