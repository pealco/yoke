// Command yoke provides a Go CLI harness for bd + PR writer/reviewer workflows.
package main

import (
	"bufio"
	"bytes"
	_ "embed"
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
	"sync"
	"time"
)

const (
	defaultBaseBranch = "main"
	defaultCheckCmd   = ".yoke/checks.sh"
	defaultPRTemplate = ".github/pull_request_template.md"
	defaultBDPrefix   = "bd"
	defaultDaemonPoll = 30 * time.Second
	reviewQueueLabel  = "yoke:in_review"
	epicPassCount     = 5
	minEpicPassCount  = 1

	epicImprovementCompleteLabel = "yoke:epic-improvement-complete"
	epicImprovementRunningLabel  = "yoke:epic-improvement-running"
	maxSummaryCommentChars       = 12000
	maxSummaryInputCharsPerPass  = 12000
	maxClarificationCommentChars = 2000
)

//go:embed prompts/epic-improvement-cycle.md
var epicImprovementPromptTemplate string

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
	BDPrefix      string
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
		bdPrefixOverride string
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
		case "--bd-prefix":
			i++
			if i >= len(args) {
				return errors.New("--bd-prefix requires a value")
			}
			normalized, err := normalizeBDPrefix(args[i])
			if err != nil {
				return err
			}
			bdPrefixOverride = normalized
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

	bdPrefix := cfg.BDPrefix
	if bdPrefixOverride != "" {
		bdPrefix = bdPrefixOverride
	}
	if bdPrefix == "" {
		bdPrefix = defaultBDPrefix
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
		if bdPrefixOverride == "" {
			selected, err := promptForBDPrefix(bdPrefix, reader)
			if err != nil {
				return err
			}
			bdPrefix = selected
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

	bdPrefix, err = normalizeBDPrefix(bdPrefix)
	if err != nil {
		return err
	}

	cfg.BDPrefix = bdPrefix
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
	note("BD prefix: " + valueOrUnset(cfg.BDPrefix))
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
	for _, name := range []string{"git", "bd"} {
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

	note("bd prefix: " + cfg.BDPrefix)

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
	bdAvailable := commandExists("bd")

	bdFocus := "unavailable"
	bdNext := "unavailable"
	if bdAvailable {
		bdFocus = issueOrNone(focusedIssueID(cfg.BDPrefix))
		bdNext = issueOrNone(nextIssueID(cfg.BDPrefix))
	}

	note("repo_root: " + root)
	note("current_branch: " + valueOrFallback(branch, "unknown"))
	note("bd_prefix: " + cfg.BDPrefix)
	note("writer_agent: " + valueOrUnset(cfg.WriterAgent))
	note("writer_agent_status: " + configuredAgentStatus(cfg.WriterAgent))
	note("writer_command: " + commandConfigStatus(cfg.WriterCmd))
	note("reviewer_agent: " + valueOrUnset(cfg.ReviewerAgent))
	note("reviewer_agent_status: " + configuredAgentStatus(cfg.ReviewerAgent))
	note("reviewer_command: " + commandConfigStatus(cfg.ReviewCmd))
	note("bd_focus: " + bdFocus)
	note("bd_next: " + bdNext)
	note("tool_git: " + availabilityLabel(commandExists("git")))
	note("tool_bd: " + availabilityLabel(bdAvailable))
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

	if !commandExists("bd") {
		return fmt.Errorf("missing required command: bd")
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
			if err := notifyDaemonMaxIterationsReached(cfg.BDPrefix, options.MaxIterations); err != nil {
				return err
			}
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
	reviewable := firstReviewableIssueID(cfg.BDPrefix)
	if reviewable != "" {
		if err := runDaemonRoleCommand("reviewer", reviewable, reviewerCmd, root, cfg.BDPrefix); err != nil {
			return "", err
		}
		return "reviewed " + reviewable, nil
	}

	inProgress, err := focusedOrInProgressIssueID(cfg.BDPrefix)
	if err != nil {
		return "", err
	}
	if inProgress != "" {
		if err := ensureIssueBranchCheckedOut(inProgress); err != nil {
			return "", err
		}
		if err := runDaemonRoleCommand("writer", inProgress, writerCmd, root, cfg.BDPrefix); err != nil {
			return "", err
		}
		return "wrote " + inProgress, nil
	}

	next := nextIssueID(cfg.BDPrefix)
	if next != "" {
		note("Daemon claiming next issue: " + next)
		if err := cmdClaim([]string{next}); err != nil {
			return "", err
		}
		return "claimed " + next, nil
	}

	return "idle", nil
}

func runDaemonRoleCommand(role, issue, shellCommand, root, bdPrefix string) error {
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
		"BD_PREFIX="+bdPrefix,
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
		return fmt.Errorf("%s command did not advance issue %s (still %s); ensure the command transitions bd state", role, issue, currentStatus)
	}

	note(fmt.Sprintf("Daemon observed %s status transition: %s -> %s", issue, previousStatus, currentStatus))
	return nil
}

func notifyDaemonMaxIterationsReached(prefix string, maxIterations int) error {
	issue, status, err := unresolvedConsensusIssue(prefix)
	if err != nil {
		return err
	}
	if issue == "" {
		return nil
	}

	note(fmt.Sprintf("warning: max iterations (%d) reached before consensus on %s (status: %s)", maxIterations, issue, status))
	note("warning: leaving PR in draft/open state for manual intervention")

	number, _, isDraft, ok := openPRForIssue(issue)
	if !ok {
		return nil
	}
	if !isDraft {
		note(fmt.Sprintf("warning: PR #%s is already ready (not draft) for %s", number, issue))
		return nil
	}

	body := formatDaemonNoConsensusPRComment(issue, status, maxIterations)
	if err := runCommand("gh", "pr", "comment", number, "--body", body); err != nil {
		note("warning: failed to post no-consensus PR comment: " + err.Error())
		return nil
	}
	note("Posted no-consensus daemon comment to PR #" + number)
	return nil
}

func unresolvedConsensusIssue(prefix string) (string, string, error) {
	reviewable := firstReviewableIssueID(prefix)
	if reviewable != "" {
		return reviewable, "in_review", nil
	}

	inProgress, err := firstIssueByStatus(prefix, "in_progress")
	if err != nil {
		return "", "", err
	}
	if inProgress != "" {
		return inProgress, "in_progress", nil
	}
	return "", "", nil
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
	branchIssue := currentBranchIssue(prefix)
	if branchIssue == "" {
		return ""
	}

	status, err := issueStatus(branchIssue)
	if err != nil {
		return ""
	}
	if status == "in_progress" {
		return branchIssue
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

type bdListIssue struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	IssueType      string   `json:"issue_type"`
	Labels         []string `json:"labels"`
	CommentCount   int      `json:"comment_count"`
	DependencyType string   `json:"dependency_type"`
}

type bdComment struct {
	ID        int    `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type clarificationContext struct {
	IssueID  string
	Title    string
	Comments []bdComment
}

func firstIssueByStatus(prefix, status string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(status), "in_review") {
		return firstReviewableIssueID(prefix), nil
	}

	output := commandCombinedOutput("bd", "list", "--status", status, "--json", "--limit", "20")
	issues, err := parseBDListIssuesJSON(output)
	if err != nil {
		return "", err
	}
	return firstMatchingIssueID(issues, prefix, status), nil
}

func parseBDListIssuesJSON(raw string) ([]bdListIssue, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var issues []bdListIssue
	if err := json.Unmarshal([]byte(trimmed), &issues); err != nil {
		return nil, fmt.Errorf("parse bd list json: %w", err)
	}
	return issues, nil
}

func parseBDCommentsJSON(raw string) ([]bdComment, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var comments []bdComment
	if err := json.Unmarshal([]byte(trimmed), &comments); err != nil {
		return nil, fmt.Errorf("parse bd comments json: %w", err)
	}
	return comments, nil
}

func firstMatchingIssueID(issues []bdListIssue, prefix, status string) string {
	targetStatus := strings.ToLower(strings.TrimSpace(status))
	for _, issue := range issues {
		issueID := strings.ToLower(strings.TrimSpace(issue.ID))
		issueStatus := workflowStatusForIssue(issue)
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
	output := commandCombinedOutput("bd", "show", issue, "--json")
	return parseIssueStatusJSON(output)
}

func issueDetails(issue string) (bdListIssue, error) {
	output := commandCombinedOutput("bd", "show", issue, "--json")
	return parseBDShowIssueJSON(output)
}

func parseIssueStatusJSON(raw string) (string, error) {
	issue, err := parseBDShowIssueJSON(raw)
	if err != nil {
		return "", err
	}

	status := workflowStatusForIssue(issue)
	if status == "" {
		return "", errors.New("issue payload missing status")
	}
	return status, nil
}

func parseBDShowIssueJSON(raw string) (bdListIssue, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return bdListIssue{}, errors.New("empty issue payload")
	}

	var listPayload []bdListIssue
	if err := json.Unmarshal([]byte(trimmed), &listPayload); err == nil {
		if len(listPayload) == 0 {
			return bdListIssue{}, errors.New("issue payload missing issue data")
		}
		return listPayload[0], nil
	}

	var singlePayload bdListIssue
	if err := json.Unmarshal([]byte(trimmed), &singlePayload); err != nil {
		return bdListIssue{}, fmt.Errorf("parse bd show json: %w", err)
	}
	if strings.TrimSpace(singlePayload.ID) == "" {
		return bdListIssue{}, errors.New("issue payload missing issue data")
	}
	return singlePayload, nil
}

func listIssuesByStatus(status string, readyOnly bool) ([]bdListIssue, error) {
	args := []string{"list", "--status", status, "--json", "--limit", "0"}
	if readyOnly {
		args = append(args, "--ready")
	}

	output := commandCombinedOutput("bd", args...)
	return parseBDListIssuesJSON(output)
}

func listChildIssues(parent string) ([]bdListIssue, error) {
	output := commandCombinedOutput("bd", "children", parent, "--json")
	return parseBDListIssuesJSON(output)
}

func listIssueDependencies(issueID string) ([]bdListIssue, error) {
	output := commandCombinedOutput("bd", "dep", "list", issueID, "--json")
	return parseBDListIssuesJSON(output)
}

func listIssueComments(issueID string) ([]bdComment, error) {
	output := commandCombinedOutput("bd", "comments", issueID, "--json")
	return parseBDCommentsJSON(output)
}

func hasOpenBlockingDependencies(dependencies []bdListIssue) bool {
	for _, dep := range dependencies {
		if !strings.EqualFold(strings.TrimSpace(dep.DependencyType), "blocks") {
			continue
		}
		if workflowStatusForIssue(dep) != "closed" {
			return true
		}
	}
	return false
}

func issueHasOpenBlockingDependencies(issueID string) (bool, error) {
	dependencies, err := listIssueDependencies(issueID)
	if err != nil {
		return false, err
	}
	return hasOpenBlockingDependencies(dependencies), nil
}

func collectDescendantIssues(root string) ([]bdListIssue, error) {
	visited := map[string]bool{}
	var descendants []bdListIssue

	var visit func(string) error
	visit = func(parent string) error {
		children, err := listChildIssues(parent)
		if err != nil {
			return err
		}
		for _, child := range children {
			id := strings.TrimSpace(child.ID)
			if id == "" || visited[id] {
				continue
			}
			visited[id] = true
			descendants = append(descendants, child)

			if err := visit(id); err != nil {
				return err
			}
		}
		return nil
	}

	if err := visit(root); err != nil {
		return nil, err
	}

	return descendants, nil
}

func collectClarificationContext(rootIssue string) ([]clarificationContext, error) {
	descendants, err := collectDescendantIssues(rootIssue)
	if err != nil {
		return nil, err
	}

	context := make([]clarificationContext, 0)
	for _, issue := range descendants {
		if !clarificationTaskReadyForAutoClose(issue) {
			continue
		}

		comments, err := listIssueComments(issue.ID)
		if err != nil {
			return nil, fmt.Errorf("load comments for %s: %w", issue.ID, err)
		}
		if len(comments) == 0 {
			continue
		}
		context = append(context, clarificationContext{
			IssueID:  issue.ID,
			Title:    issue.Title,
			Comments: comments,
		})
	}

	return context, nil
}

func isClarificationNeededTitle(title string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "clarification needed:")
}

func clarificationTaskReadyForAutoClose(issue bdListIssue) bool {
	if !isClarificationNeededTitle(issue.Title) {
		return false
	}
	if issue.CommentCount <= 0 {
		return false
	}
	return workflowStatusForIssue(issue) != "closed"
}

func closeClarificationTasksWithComments(rootIssue string) (int, error) {
	descendants, err := collectDescendantIssues(rootIssue)
	if err != nil {
		return 0, err
	}

	closed := 0
	for _, issue := range descendants {
		if !clarificationTaskReadyForAutoClose(issue) {
			continue
		}
		claimNote("Auto-closing clarification task with comments: " + issue.ID)
		if err := runCommand("bd", "close", issue.ID, "--reason", "clarified-by-comment"); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}

func pickEpicChildToClaim(descendants, inProgress, ready []bdListIssue) (string, bool) {
	workItems := map[string]bdListIssue{}
	for _, issue := range descendants {
		id := strings.TrimSpace(issue.ID)
		if id == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(issue.IssueType), "epic") {
			continue
		}
		workItems[id] = issue
	}

	if len(workItems) == 0 {
		return "", true
	}

	for _, issue := range inProgress {
		id := strings.TrimSpace(issue.ID)
		if _, ok := workItems[id]; ok {
			return id, false
		}
	}

	for _, issue := range ready {
		id := strings.TrimSpace(issue.ID)
		if _, ok := workItems[id]; ok {
			return id, false
		}
	}

	for _, issue := range workItems {
		if workflowStatusForIssue(issue) != "closed" {
			return "", false
		}
	}

	return "", true
}

func resolveClaimIssue(root string, cfg config, issue string, passLimit int) (string, bool, error) {
	claimNote("Loading issue details for " + issue)
	details, err := issueDetails(issue)
	if err != nil {
		return "", false, err
	}
	claimNote(fmt.Sprintf("Issue %s resolved as type=%s status=%s", details.ID, details.IssueType, workflowStatusForIssue(details)))
	if !strings.EqualFold(strings.TrimSpace(details.IssueType), "epic") {
		claimNote("Issue is not an epic; proceeding with direct claim.")
		return issue, false, nil
	}
	if workflowStatusForIssue(details) == "closed" {
		claimNote("Epic is already closed; no child task to claim.")
		return "", true, nil
	}
	claimNote(fmt.Sprintf("Issue is an epic; running epic improvement cycle (limit=%d pass(es)) before selecting a child task.", passLimit))
	if err := runEpicImprovementCycle(root, cfg, details, passLimit); err != nil {
		return "", false, err
	}
	claimNote("Auto-resolving clarification tasks that have comments.")
	autoClosedCount, err := closeClarificationTasksWithComments(issue)
	if err != nil {
		return "", false, err
	}
	if autoClosedCount == 0 {
		claimNote("No clarification tasks required auto-close.")
	} else {
		claimNote(fmt.Sprintf("Auto-closed %d clarification task(s) from user comments.", autoClosedCount))
	}
	claimNote("Collecting epic descendants for claim selection.")

	descendants, err := collectDescendantIssues(issue)
	if err != nil {
		return "", false, err
	}
	claimNote(fmt.Sprintf("Collected %d descendant issue(s).", len(descendants)))

	claimNote("Loading in-progress issues for possible resume.")
	inProgress, err := listIssuesByStatus("in_progress", false)
	if err != nil {
		return "", false, err
	}
	claimNote(fmt.Sprintf("Found %d in-progress issue(s).", len(inProgress)))
	filteredInProgress := make([]bdListIssue, 0, len(inProgress))
	skippedInProgress := make([]string, 0)
	for _, candidate := range inProgress {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		hasOpenDeps, err := issueHasOpenBlockingDependencies(id)
		if err != nil {
			return "", false, err
		}
		if hasOpenDeps {
			skippedInProgress = append(skippedInProgress, id)
			continue
		}
		filteredInProgress = append(filteredInProgress, candidate)
	}
	if len(skippedInProgress) > 0 {
		claimNote("Skipping blocked in-progress issue(s): " + strings.Join(skippedInProgress, ", "))
	}
	claimNote(fmt.Sprintf("Claimable in-progress issue(s): %d", len(filteredInProgress)))
	claimNote("Loading ready open issues for fallback selection.")
	ready, err := listIssuesByStatus("open", true)
	if err != nil {
		return "", false, err
	}
	claimNote(fmt.Sprintf("Found %d ready open issue(s).", len(ready)))

	target, epicComplete := pickEpicChildToClaim(descendants, filteredInProgress, ready)
	if target != "" {
		claimNote("Selected claimable child task: " + target)
		return target, false, nil
	}
	if epicComplete {
		claimNote("All non-epic descendants are closed; closing epic.")
		currentStatus, err := issueStatus(issue)
		if err != nil {
			return "", false, err
		}
		if currentStatus != "closed" {
			claimNote("Closing epic " + issue + " with reason all-child-tasks-closed.")
			if err := runCommand("bd", "close", issue, "--reason", "all-child-tasks-closed"); err != nil {
				return "", false, err
			}
		} else {
			claimNote("Epic already closed; no close command needed.")
		}
		return "", true, nil
	}

	claimNote("No claimable child task found; remaining work is blocked or already claimed.")
	return "", false, fmt.Errorf("epic %s has no claimable child tasks (all remaining children are blocked or already claimed)", issue)
}

type epicImprovementPassReport struct {
	Pass    int
	Role    string
	AgentID string
	Output  string
}

func runEpicImprovementCycle(root string, cfg config, epic bdListIssue, passLimit int) error {
	if passLimit < minEpicPassCount || passLimit > epicPassCount {
		return fmt.Errorf("improvement pass limit must be between %d and %d", minEpicPassCount, epicPassCount)
	}
	if strings.TrimSpace(epicImprovementPromptTemplate) == "" {
		return errors.New("epic improvement prompt template is empty")
	}
	claimNote("Checking for clarification tasks with comments before starting passes.")
	clarificationContext, err := collectClarificationContext(epic.ID)
	if err != nil {
		return err
	}
	if hasLabel(epic.Labels, epicImprovementCompleteLabel) {
		if len(clarificationContext) == 0 {
			claimNote("Epic improvement cycle already complete (label present); skipping rerun.")
			return nil
		}
		claimNote(fmt.Sprintf("Epic improvement already marked complete, but found %d clarification task(s) with comments; re-running improvement cycle.", len(clarificationContext)))
	}
	if len(clarificationContext) == 0 {
		claimNote("No clarification tasks with comments found.")
	} else {
		claimNote(fmt.Sprintf("Found %d clarification task(s) with comments; injecting context into prompts.", len(clarificationContext)))
		for _, item := range clarificationContext {
			claimNote(fmt.Sprintf("Loaded clarification context: %s (%d comment(s))", item.IssueID, len(item.Comments)))
		}
	}

	claimNote(fmt.Sprintf("Starting epic improvement cycle for %s (%d pass(es)).", epic.ID, passLimit))
	reportsDir := filepath.Join(root, ".yoke", "epic-improvement-reports", sanitizePathSegment(epic.ID))
	claimNote("Improvement reports directory: " + reportsDir)
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return err
	}
	claimNote("Marking epic as improvement-running.")
	if err := runCommand("bd", "update", epic.ID, "--add-label", epicImprovementRunningLabel); err != nil {
		return err
	}

	reports := make([]epicImprovementPassReport, 0, passLimit)
	for pass := 1; pass <= passLimit; pass++ {
		role := roleForPass(pass)
		agentID, err := agentIDForRole(cfg, role)
		if err != nil {
			return err
		}
		claimNote(fmt.Sprintf("Improvement pass %d/%d starting (role=%s, agent=%s).", pass, passLimit, role, agentID))

		prompt := buildEpicImprovementPassPrompt(epic.ID, pass, passLimit, role, clarificationContext)
		output, runErr := runAgentPrompt(agentID, root, prompt, []string{
			"ISSUE_ID=" + epic.ID,
			"ROOT_DIR=" + root,
			"BD_PREFIX=" + cfg.BDPrefix,
			"YOKE_ROLE=" + role,
			"YOKE_EPIC_IMPROVEMENT_PASS=" + strconv.Itoa(pass),
		}, fmt.Sprintf("[claim][pass %d/%d %s] ", pass, passLimit, role))

		reportPath := filepath.Join(reportsDir, fmt.Sprintf("pass-%02d-%s.md", pass, role))
		if err := writeEpicImprovementPassReport(reportPath, epic.ID, pass, role, agentID, output, runErr); err != nil {
			return err
		}
		claimNote("Saved improvement pass report: " + reportPath)
		if runErr != nil {
			claimNote(fmt.Sprintf("Improvement pass %d failed; see report: %s", pass, reportPath))
			return fmt.Errorf("epic improvement pass %d (%s) failed: %w (report: %s)", pass, role, runErr, reportPath)
		}
		claimNote(fmt.Sprintf("Improvement pass %d/%d completed.", pass, passLimit))

		reports = append(reports, epicImprovementPassReport{
			Pass:    pass,
			Role:    role,
			AgentID: agentID,
			Output:  output,
		})
	}

	summaryAgentID, err := agentIDForRole(cfg, "reviewer")
	if err != nil {
		return err
	}
	claimNote("Generating final improvement summary with reviewer agent " + summaryAgentID + ".")
	summaryPrompt := buildEpicImprovementSummaryPrompt(epic, reports)
	summary, runErr := runAgentPrompt(summaryAgentID, root, summaryPrompt, []string{
		"ISSUE_ID=" + epic.ID,
		"ROOT_DIR=" + root,
		"BD_PREFIX=" + cfg.BDPrefix,
		"YOKE_ROLE=reviewer",
		"YOKE_EPIC_IMPROVEMENT_SUMMARY=1",
	}, "[claim][summary] ")
	summaryPath := filepath.Join(reportsDir, "summary.md")
	if err := writeEpicImprovementSummary(summaryPath, epic.ID, summaryAgentID, summary, runErr); err != nil {
		return err
	}
	claimNote("Saved improvement summary report: " + summaryPath)
	if runErr != nil {
		claimNote("Improvement summary generation failed; see report: " + summaryPath)
		return fmt.Errorf("epic improvement summary failed: %w (report: %s)", runErr, summaryPath)
	}

	claimNote("Posting improvement summary comment to epic " + epic.ID + ".")
	comment := formatEpicImprovementSummaryComment(epic, summary, passLimit, reportsDir)
	if err := runCommand("bd", "comments", "add", epic.ID, comment); err != nil {
		return err
	}
	claimNote("Marking epic improvement complete and clearing running label.")
	if err := runCommand("bd", "update", epic.ID,
		"--add-label", epicImprovementCompleteLabel,
		"--remove-label", epicImprovementRunningLabel,
	); err != nil {
		return err
	}

	note(fmt.Sprintf("Completed epic improvement cycle for %s; reports saved in %s", epic.ID, reportsDir))
	return nil
}

func roleForPass(pass int) string {
	if pass%2 == 1 {
		return "writer"
	}
	return "reviewer"
}

func agentIDForRole(cfg config, role string) (string, error) {
	switch role {
	case "writer":
		if strings.TrimSpace(cfg.WriterAgent) != "" {
			return cfg.WriterAgent, nil
		}
	case "reviewer":
		if strings.TrimSpace(cfg.ReviewerAgent) != "" {
			return cfg.ReviewerAgent, nil
		}
		if strings.TrimSpace(cfg.WriterAgent) != "" {
			return cfg.WriterAgent, nil
		}
	}
	return "", fmt.Errorf("no %s agent configured; run yoke init or set agent config in .yoke/config.sh", role)
}

func agentBinaryForID(agentID string) (string, string, error) {
	normalized, ok := normalizeAgentID(agentID)
	if !ok {
		return "", "", fmt.Errorf("unsupported agent id: %s", agentID)
	}
	for _, spec := range supportedAgents {
		if spec.ID != normalized {
			continue
		}
		for _, binary := range spec.Binaries {
			if commandExists(binary) {
				return normalized, binary, nil
			}
		}
		break
	}
	return "", "", fmt.Errorf("agent %s is not available on PATH", normalized)
}

func runAgentPrompt(agentID, root, prompt string, extraEnv []string, streamPrefix string) (string, error) {
	normalized, binary, err := agentBinaryForID(agentID)
	if err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	switch normalized {
	case "codex":
		cmd = exec.Command(binary, "exec", "--full-auto", "--cd", root, prompt)
	case "claude":
		cmd = exec.Command(binary, "--print", "--permission-mode", "bypassPermissions", prompt)
	default:
		return "", fmt.Errorf("unsupported agent id: %s", normalized)
	}
	cmd.Dir = root
	cmd.Env = append(os.Environ(), extraEnv...)

	var combined synchronizedBuffer
	stdoutStream := io.MultiWriter(&combined, newLinePrefixWriter(os.Stdout, streamPrefix))
	stderrPrefix := streamPrefix
	if strings.TrimSpace(stderrPrefix) == "" {
		stderrPrefix = "[agent][stderr] "
	} else {
		stderrPrefix += "[stderr] "
	}
	stderrStream := io.MultiWriter(&combined, newLinePrefixWriter(os.Stdout, stderrPrefix))
	cmd.Stdout = stdoutStream
	cmd.Stderr = stderrStream

	runErr := cmd.Run()
	return strings.TrimSpace(combined.String()), runErr
}

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type linePrefixWriter struct {
	mu        sync.Mutex
	dst       io.Writer
	prefix    string
	lineStart bool
}

func newLinePrefixWriter(dst io.Writer, prefix string) *linePrefixWriter {
	return &linePrefixWriter{
		dst:       dst,
		prefix:    prefix,
		lineStart: true,
	}
}

func (w *linePrefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written := 0
	for len(p) > 0 {
		if w.lineStart {
			if _, err := io.WriteString(w.dst, w.prefix); err != nil {
				return written, err
			}
			w.lineStart = false
		}

		newline := bytes.IndexByte(p, '\n')
		if newline == -1 {
			n, err := w.dst.Write(p)
			written += n
			return written, err
		}

		chunk := p[:newline+1]
		n, err := w.dst.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		w.lineStart = true
		p = p[newline+1:]
	}

	return written, nil
}

func buildEpicImprovementPassPrompt(epicID string, pass, total int, role string, clarifications []clarificationContext) string {
	replaced := strings.ReplaceAll(epicImprovementPromptTemplate, "$EPIC_ID", epicID)
	clarificationBlock := buildClarificationPromptBlock(clarifications)
	if clarificationBlock == "" {
		clarificationBlock = "No clarification-task comments were found."
	}
	return strings.TrimSpace(fmt.Sprintf(
		`You are the %s agent for epic %s.
This is epic improvement pass %d of %d.
Clarification context (resolved by user comments on "Clarification needed" tasks):

%s

Apply the following improvement protocol exactly and emit the report in the specified report format:

%s`,
		role, epicID, pass, total, clarificationBlock, replaced,
	))
}

func buildClarificationPromptBlock(clarifications []clarificationContext) string {
	if len(clarifications) == 0 {
		return ""
	}

	var body strings.Builder
	for _, item := range clarifications {
		body.WriteString(fmt.Sprintf("- %s: %s\n", item.IssueID, strings.TrimSpace(item.Title)))
		for _, comment := range item.Comments {
			author := strings.TrimSpace(comment.Author)
			if author == "" {
				author = "unknown"
			}
			timestamp := strings.TrimSpace(comment.CreatedAt)
			if timestamp == "" {
				timestamp = "unknown-time"
			}
			text := truncateForPrompt(strings.TrimSpace(comment.Text), maxClarificationCommentChars)
			body.WriteString(fmt.Sprintf("  - [%s @ %s] %s\n", author, timestamp, text))
		}
	}
	return strings.TrimSpace(body.String())
}

func buildEpicImprovementSummaryPrompt(epic bdListIssue, reports []epicImprovementPassReport) string {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Epic: %s\n", epic.ID))
	body.WriteString(fmt.Sprintf("Title: %s\n\n", strings.TrimSpace(epic.Title)))
	body.WriteString("Summarize the five pass reports below into one concise final report.\n")
	body.WriteString("Use sections:\n")
	body.WriteString("1) Improvements made\n")
	body.WriteString("2) Remaining risks/questions\n")
	body.WriteString("3) Most critical dependency chains\n")
	body.WriteString("4) Recommended next implementation steps\n\n")

	for _, report := range reports {
		body.WriteString(fmt.Sprintf("## Pass %d (%s via %s)\n", report.Pass, report.Role, report.AgentID))
		body.WriteString(truncateForPrompt(report.Output, maxSummaryInputCharsPerPass))
		body.WriteString("\n\n")
	}
	return body.String()
}

func truncateForPrompt(value string, maxChars int) string {
	trimmed := strings.TrimSpace(value)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "\n...[truncated]..."
}

func writeEpicImprovementPassReport(path, epicID string, pass int, role, agentID, output string, runErr error) error {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("# Epic Improvement Pass %d\n\n", pass))
	body.WriteString(fmt.Sprintf("- Epic: `%s`\n", epicID))
	body.WriteString(fmt.Sprintf("- Role: `%s`\n", role))
	body.WriteString(fmt.Sprintf("- Agent: `%s`\n", agentID))
	body.WriteString(fmt.Sprintf("- Timestamp: `%s`\n", time.Now().Format(time.RFC3339)))
	if runErr != nil {
		body.WriteString(fmt.Sprintf("- Exit: error (`%s`)\n", runErr))
	} else {
		body.WriteString("- Exit: success\n")
	}
	body.WriteString("\n## Output\n\n")
	body.WriteString(output)
	body.WriteString("\n")
	return os.WriteFile(path, []byte(body.String()), 0o644)
}

func writeEpicImprovementSummary(path, epicID, agentID, summary string, runErr error) error {
	var body strings.Builder
	body.WriteString("# Epic Improvement Summary\n\n")
	body.WriteString(fmt.Sprintf("- Epic: `%s`\n", epicID))
	body.WriteString(fmt.Sprintf("- Agent: `%s`\n", agentID))
	body.WriteString(fmt.Sprintf("- Timestamp: `%s`\n", time.Now().Format(time.RFC3339)))
	if runErr != nil {
		body.WriteString(fmt.Sprintf("- Exit: error (`%s`)\n", runErr))
	} else {
		body.WriteString("- Exit: success\n")
	}
	body.WriteString("\n## Output\n\n")
	body.WriteString(summary)
	body.WriteString("\n")
	return os.WriteFile(path, []byte(body.String()), 0o644)
}

func formatEpicImprovementSummaryComment(epic bdListIssue, summary string, passCount int, reportsDir string) string {
	trimmedSummary := truncateForPrompt(summary, maxSummaryCommentChars)
	lines := []string{
		"## Epic Improvement Cycle Complete",
		"",
		"- Epic: `" + sanitizeCommentLine(epic.ID) + "`",
		"- Passes: " + strconv.Itoa(passCount),
		"- Process: writer/reviewer alternating",
		"",
		"### Agent Summary",
		trimmedSummary,
		"",
		"_Local reports saved at: `" + sanitizeCommentLine(reportsDir) + "`_",
	}
	return strings.Join(lines, "\n")
}

func sanitizePathSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_").Replace(trimmed)
}

func workflowStatusForIssue(issue bdListIssue) string {
	status := strings.ToLower(strings.TrimSpace(issue.Status))
	if status == "blocked" && hasLabel(issue.Labels, reviewQueueLabel) {
		return "in_review"
	}
	return status
}

func hasLabel(labels []string, target string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label), target) {
			return true
		}
	}
	return false
}

func cmdClaim(args []string) error {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			printClaimUsage()
			return nil
		}
	}
	claimNote("Starting claim command.")
	issueArg, improvementPassLimit, err := parseClaimArgs(args)
	if err != nil {
		return err
	}
	claimNote(fmt.Sprintf("Epic improvement pass limit set to %d.", improvementPassLimit))

	root, err := ensureRepoRoot()
	if err != nil {
		return err
	}
	claimNote("Resolved repository root: " + root)
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	claimNote("Loaded config with bd prefix: " + cfg.BDPrefix)
	if !commandExists("bd") {
		return fmt.Errorf("missing required command: bd")
	}
	claimNote("Verified required command: bd")

	issue := issueArg
	if issue != "" {
		claimNote("Using explicit issue argument: " + issue)
	}

	if issue == "" {
		claimNote("No issue argument provided; selecting next ready open issue from bd.")
		issue = nextIssueID(cfg.BDPrefix)
	}
	if issue == "" {
		return errors.New("no issue provided and bd ready returned nothing")
	}
	claimNote("Requested claim target: " + issue)

	requestedIssue := issue
	claimNote("Resolving target with epic-aware claim logic.")
	resolvedIssue, epicCompleted, err := resolveClaimIssue(root, cfg, issue, improvementPassLimit)
	if err != nil {
		return err
	}
	if epicCompleted {
		claimNote("Requested epic has no remaining open child tasks.")
		note("Epic " + requestedIssue + " is complete; closed epic.")
		return nil
	}
	issue = resolvedIssue
	if requestedIssue != issue {
		note("Epic " + requestedIssue + " -> claiming child task " + issue)
	}

	claimNote("Transitioning issue to in_progress and removing review queue label if present.")
	if err := runCommand("bd", "update", issue, "--status", "in_progress", "--remove-label", reviewQueueLabel); err != nil {
		return err
	}
	claimNote("Issue state updated successfully.")

	branch := branchForIssue(issue)
	claimNote("Preparing git branch: " + branch)
	if refExists("refs/heads/" + branch) {
		claimNote("Branch exists locally; switching.")
		if err := runCommand("git", "switch", branch); err != nil {
			return err
		}
	} else {
		claimNote("Branch does not exist; creating and switching.")
		if err := runCommand("git", "switch", "-c", branch); err != nil {
			return err
		}
	}
	claimNote("Branch is ready for development.")

	note(fmt.Sprintf("Claimed %s on branch %s", issue, branch))
	note(fmt.Sprintf("Next: yoke submit %s --done \"...\" --remaining \"...\"", issue))
	return nil
}

func parseClaimArgs(args []string) (issue string, improvementPassLimit int, err error) {
	issue = ""
	improvementPassLimit = epicPassCount

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--improvement-passes":
			i++
			if i >= len(args) {
				return "", 0, errors.New("--improvement-passes requires a value")
			}
			passLimit, convErr := strconv.Atoi(args[i])
			if convErr != nil || passLimit < minEpicPassCount || passLimit > epicPassCount {
				return "", 0, fmt.Errorf("--improvement-passes must be an integer between %d and %d", minEpicPassCount, epicPassCount)
			}
			improvementPassLimit = passLimit
		default:
			if strings.HasPrefix(arg, "-") {
				return "", 0, fmt.Errorf("unknown claim argument: %s", arg)
			}
			if issue != "" {
				return "", 0, errors.New("usage: yoke claim [<prefix>-issue-id] [--improvement-passes N]")
			}
			issue = arg
		}
	}

	return issue, improvementPassLimit, nil
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
		noPRNote  bool
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
		case "--no-pr-comment":
			noPRNote = true
		case "-h", "--help":
			printSubmitUsage()
			return nil
		default:
			if looksLikeIssueID(arg, cfg.BDPrefix) {
				if issue != "" {
					return errors.New("multiple issue ids provided")
				}
				issue = arg
				continue
			}
			return fmt.Errorf("unknown submit argument: %s", arg)
		}
	}

	if !commandExists("bd") {
		return fmt.Errorf("missing required command: bd")
	}
	if doneText == "" {
		return errors.New("--done is required")
	}
	if remaining == "" {
		return errors.New("--remaining is required")
	}

	if issue == "" {
		issue = currentBranchIssue(cfg.BDPrefix)
	}
	if issue == "" {
		return fmt.Errorf("could not infer issue id from branch; pass %s-xxxx explicitly", cfg.BDPrefix)
	}

	checkCommand := cfg.CheckCmd
	if checks != "" {
		checkCommand = checks
	}
	if err := runChecks(root, checkCommand); err != nil {
		return err
	}

	handoffComment := formatIssueHandoffComment(doneText, remaining, decision, uncertain, checkCommand)
	if err := runCommand("bd", "comments", "add", issue, handoffComment); err != nil {
		return err
	}

	if err := runCommand("bd", "update", issue, "--status", "blocked", "--add-label", reviewQueueLabel); err != nil {
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
	if !noPRNote {
		postSubmitPRComment(issue, doneText, remaining, decision, uncertain, checkCommand)
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
		noPRNote     bool
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
		case "--no-pr-comment":
			noPRNote = true
		case "-h", "--help":
			printReviewUsage()
			return nil
		default:
			if looksLikeIssueID(arg, cfg.BDPrefix) {
				if issue != "" {
					return errors.New("multiple issue ids provided")
				}
				issue = arg
				continue
			}
			return fmt.Errorf("unknown review argument: %s", arg)
		}
	}

	if !commandExists("bd") {
		return fmt.Errorf("missing required command: bd")
	}

	if issue == "" {
		issue = firstReviewableIssueID(cfg.BDPrefix)
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
			"BD_PREFIX="+cfg.BDPrefix,
			"YOKE_ROLE=reviewer",
		)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	if noteText != "" {
		if err := runCommand("bd", "comments", "add", issue, noteText); err != nil {
			return err
		}
	}

	switch action {
	case "approve":
		if err := runCommand("bd", "close", issue, "--reason", "approved-by-yoke-review"); err != nil {
			return err
		}
		currentStatus, err := issueStatus(issue)
		if err != nil {
			return err
		}
		if currentStatus != "closed" {
			return fmt.Errorf("bd close did not close %s (current status: %s)", issue, currentStatus)
		}
		if err := ensureIssuePRReady(issue); err != nil {
			return err
		}
		note("Approved " + issue)
	case "reject":
		if rejectReason != "" {
			if err := runCommand("bd", "comments", "add", issue, "Reviewer rejection: "+rejectReason); err != nil {
				return err
			}
		}
		if err := runCommand("bd", "update", issue, "--status", "in_progress", "--remove-label", reviewQueueLabel); err != nil {
			return err
		}
		currentStatus, err := issueStatus(issue)
		if err != nil {
			return err
		}
		if currentStatus != "in_progress" {
			return fmt.Errorf("bd update did not return %s to in_progress (current status: %s)", issue, currentStatus)
		}
		note("Rejected " + issue)
	default:
		if err := runCommand("bd", "show", issue); err != nil {
			return err
		}
		note("Next:")
		note("  yoke review " + issue + " --approve")
		note("  yoke review " + issue + " --reject \"reason\"")
	}
	if !noPRNote && (action != "" || noteText != "") {
		postReviewPRComment(issue, action, rejectReason, noteText, runAgent)
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
		BDPrefix:      defaultBDPrefix,
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
		case "YOKE_BD_PREFIX":
			cfg.BDPrefix = value
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

	normalizedPrefix, err := normalizeBDPrefix(cfg.BDPrefix)
	if err != nil {
		return cfg, err
	}
	cfg.BDPrefix = normalizedPrefix

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

# Prefix used for bd issue IDs (example: bd-a1b2).
YOKE_BD_PREFIX=%s

# Selected coding agent for writing (codex or claude).
YOKE_WRITER_AGENT=%s

# Optional writer command for yoke daemon loops.
# Runs with ISSUE_ID, ROOT_DIR, BD_PREFIX, and YOKE_ROLE=writer.
# Expected behavior: implement the issue and transition state via yoke submit.
YOKE_WRITER_CMD=%s

# Selected coding agent for reviewing (codex or claude).
YOKE_REVIEWER_AGENT=%s

# Optional reviewer agent command. Runs when using: yoke review --agent
# and yoke daemon. Runs with ISSUE_ID, ROOT_DIR, BD_PREFIX, and YOKE_ROLE=reviewer.
# Expected behavior for daemon mode: execute yoke review --approve or --reject.
# Example:
# YOKE_REVIEW_CMD='codex exec "Review $ISSUE_ID and run yoke review $ISSUE_ID --approve or --reject with reason"'
YOKE_REVIEW_CMD=%s

# Pull request template path.
YOKE_PR_TEMPLATE=%s
`,
		quoteShell(cfg.BaseBranch),
		quoteShell(cfg.CheckCmd),
		quoteShell(cfg.BDPrefix),
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

func normalizeBDPrefix(input string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return defaultBDPrefix, nil
	}

	validPrefix := regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
	if !validPrefix.MatchString(value) {
		return "", fmt.Errorf("invalid bd prefix %q: use letters, numbers, '.', '_' or '-', and avoid trailing separators", input)
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

func promptForBDPrefix(current string, reader *bufio.Reader) (string, error) {
	defaultPrefix, err := normalizeBDPrefix(current)
	if err != nil {
		defaultPrefix = defaultBDPrefix
	}

	for {
		fmt.Printf("BD issue prefix [%s]: ", defaultPrefix)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return defaultPrefix, nil
		}

		normalized, normErr := normalizeBDPrefix(trimmed)
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
	normalized, err := normalizeBDPrefix(prefix)
	if err != nil {
		normalized = defaultBDPrefix
	}
	return regexp.MustCompile(regexp.QuoteMeta(normalized) + `-[a-z0-9]+(?:\.[a-z0-9]+)*`)
}

func extractIssueID(s, prefix string) string {
	return issuePatternForPrefix(prefix).FindString(strings.ToLower(s))
}

func looksLikeIssueID(value, prefix string) bool {
	pattern := issuePatternForPrefix(prefix)
	return pattern.FindString(strings.ToLower(value)) == strings.ToLower(value)
}

func nextIssueID(prefix string) string {
	output := commandCombinedOutput("bd", "list", "--status", "open", "--ready", "--json", "--limit", "20")
	issues, err := parseBDListIssuesJSON(output)
	if err != nil {
		return ""
	}
	return firstMatchingIssueID(issues, prefix, "open")
}

func firstReviewableIssueID(prefix string) string {
	output := commandCombinedOutput("bd", "list", "--status", "blocked", "--label", reviewQueueLabel, "--json", "--limit", "20")
	issues, err := parseBDListIssuesJSON(output)
	if err != nil {
		return ""
	}
	return firstMatchingIssueID(issues, prefix, "in_review")
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
	output := commandCombinedOutput("bd", "show", issue, "--json")
	parsed, err := parseBDShowIssueJSON(output)
	if err == nil && strings.TrimSpace(parsed.Title) != "" {
		return strings.TrimSpace(parsed.Title)
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

	if number, _, _, ok := openPRForBranch(branch); ok {
		note(fmt.Sprintf("PR #%s already exists for %s.", number, branch))
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

type prListEntry struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	IsDraft bool   `json:"isDraft"`
}

func openPRForIssue(issue string) (string, string, bool, bool) {
	branch := branchForIssue(issue)
	return openPRForBranch(branch)
}

func openPRForBranch(branch string) (string, string, bool, bool) {
	if strings.TrimSpace(branch) == "" {
		return "", "", false, false
	}
	if !commandExists("gh") || !hasOriginRemote() {
		return "", "", false, false
	}

	output := strings.TrimSpace(commandCombinedOutput(
		"gh", "pr", "list",
		"--head", branch,
		"--state", "open",
		"--json", "number,url,isDraft",
	))
	return parseOpenPRFromListJSON(output)
}

func parseOpenPRFromListJSON(raw string) (string, string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return "", "", false, false
	}

	var list []prListEntry
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return "", "", false, false
	}
	if len(list) == 0 || list[0].Number <= 0 {
		return "", "", false, false
	}
	return strconv.Itoa(list[0].Number), strings.TrimSpace(list[0].URL), list[0].IsDraft, true
}

func postSubmitPRComment(issue, doneText, remaining, decision, uncertain, checks string) {
	number, _, _, ok := openPRForIssue(issue)
	if !ok {
		note("warning: no open PR found for issue branch; skipping writer handoff PR comment")
		return
	}

	body := formatWriterPRComment(issue, doneText, remaining, decision, uncertain, checks)
	if err := runCommand("gh", "pr", "comment", number, "--body", body); err != nil {
		note("warning: failed to post writer handoff PR comment: " + err.Error())
		return
	}
	note("Posted writer handoff comment to PR #" + number)
}

func postReviewPRComment(issue, action, rejectReason, noteText string, runAgent bool) {
	number, _, _, ok := openPRForIssue(issue)
	if !ok {
		note("warning: no open PR found for issue branch; skipping reviewer PR comment")
		return
	}

	body := formatReviewerPRComment(issue, action, rejectReason, noteText, runAgent)
	if err := runCommand("gh", "pr", "comment", number, "--body", body); err != nil {
		note("warning: failed to post reviewer PR comment: " + err.Error())
		return
	}
	note("Posted reviewer comment to PR #" + number)
}

func formatWriterPRComment(issue, doneText, remaining, decision, uncertain, checks string) string {
	lines := []string{
		"## Writer -> Reviewer Handoff",
		"",
		"- Issue: `" + sanitizeCommentLine(issue) + "`",
		"- Done: " + sanitizeCommentLine(doneText),
		"- Remaining: " + sanitizeCommentLine(remaining),
	}
	if strings.TrimSpace(decision) != "" {
		lines = append(lines, "- Decision: "+sanitizeCommentLine(decision))
	}
	if strings.TrimSpace(uncertain) != "" {
		lines = append(lines, "- Uncertain: "+sanitizeCommentLine(uncertain))
	}
	lines = append(lines, "- Checks: `"+sanitizeCommentLine(checks)+"` passed")
	lines = append(lines, "")
	lines = append(lines, "_Posted automatically by `yoke submit`._")
	return strings.Join(lines, "\n")
}

func formatIssueHandoffComment(doneText, remaining, decision, uncertain, checks string) string {
	lines := []string{
		"Writer handoff:",
		"- Done: " + sanitizeCommentLine(doneText),
		"- Remaining: " + sanitizeCommentLine(remaining),
		"- Checks: `" + sanitizeCommentLine(checks) + "` passed",
	}
	if strings.TrimSpace(decision) != "" {
		lines = append(lines, "- Decision: "+sanitizeCommentLine(decision))
	}
	if strings.TrimSpace(uncertain) != "" {
		lines = append(lines, "- Uncertain: "+sanitizeCommentLine(uncertain))
	}
	return strings.Join(lines, "\n")
}

func formatReviewerPRComment(issue, action, rejectReason, noteText string, runAgent bool) string {
	decision := "note"
	if strings.TrimSpace(action) != "" {
		decision = strings.TrimSpace(action)
	}

	lines := []string{
		"## Reviewer Update",
		"",
		"- Issue: `" + sanitizeCommentLine(issue) + "`",
		"- Decision: " + sanitizeCommentLine(decision),
	}
	if decision == "reject" && strings.TrimSpace(rejectReason) != "" {
		lines = append(lines, "- Reject reason: "+sanitizeCommentLine(rejectReason))
	}
	if strings.TrimSpace(noteText) != "" {
		lines = append(lines, "- Note: "+sanitizeCommentLine(noteText))
	}
	if runAgent {
		lines = append(lines, "- Reviewer command: executed")
	}
	lines = append(lines, "")
	lines = append(lines, "_Posted automatically by `yoke review`._")
	return strings.Join(lines, "\n")
}

func formatDaemonNoConsensusPRComment(issue, status string, maxIterations int) string {
	lines := []string{
		"## Daemon Notice",
		"",
		"- Issue: `" + sanitizeCommentLine(issue) + "`",
		"- Status: " + sanitizeCommentLine(status),
		"- Outcome: max daemon iterations reached without writer/reviewer consensus",
		"- Iterations: " + strconv.Itoa(maxIterations),
		"- PR state: left in draft for manual intervention",
		"",
		"_Posted automatically by `yoke daemon`._",
	}
	return strings.Join(lines, "\n")
}

func ensureIssuePRReady(issue string) error {
	number, _, isDraft, ok := openPRForIssue(issue)
	if !ok {
		note("warning: no open PR found for issue branch; skipping ready-for-review transition")
		return nil
	}
	if !isDraft {
		return nil
	}
	if err := runCommand("gh", "pr", "ready", number); err != nil {
		return fmt.Errorf("failed to mark PR #%s ready after approval: %w", number, err)
	}
	note("Marked PR #" + number + " ready for review")
	return nil
}

func sanitizeCommentLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
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

func claimNote(msg string) {
	note("[claim] " + msg)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "yoke: %s\n", err)
	os.Exit(1)
}

func printUsage() {
	fmt.Print(`yoke: agent-first bd + PR harness

Purpose:
  Coordinate writer/reviewer workflows for coding agents using bd state transitions
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
  daemon  Run continuous writer/reviewer automation loop over bd issue states.
  claim   Start work on an issue (bd update --status in_progress + branch switch/create).
  submit  Run checks, add handoff comment, move issue to review queue, and open/update PR workflow.
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
  3) In interactive terminals, prompts for bd issue prefix selection.
  4) In interactive terminals, prompts for writer and reviewer selection.
     Writer and reviewer may be the same agent.
  5) Writes selections to .yoke/config.sh.

Options:
  --writer-agent codex|claude     Set writer agent explicitly.
  --reviewer-agent codex|claude   Set reviewer agent explicitly.
  --bd-prefix PREFIX              Set bd issue prefix explicitly (default: bd).
  --no-prompt                     Do not prompt; auto-select detected defaults.

Examples:
  yoke init
  yoke init --writer-agent codex --reviewer-agent codex
  yoke init --no-prompt --writer-agent codex --reviewer-agent claude --bd-prefix bd

Outputs:
  Updates .yoke/config.sh keys:
  - YOKE_BD_PREFIX
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
  - Required binaries: git, bd
  - Optional binary: gh
  - Config file presence: .yoke/config.sh
  - Configured bd issue prefix
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
  - bd_prefix: configured issue prefix from YOKE_BD_PREFIX
  - writer_agent / reviewer_agent: configured agent ids (or unset)
  - writer_agent_status / reviewer_agent_status: binary availability summary
  - writer_command / reviewer_command: daemon command readiness
  - bd_focus: focused in-progress issue inferred from current branch (or none/unavailable)
  - bd_next: next ready open issue from bd (or none/unavailable)
  - tool_git / tool_bd / tool_gh: command availability

Usage guidance for agents:
  1) Run yoke status before claim/submit/review to confirm context.
  2) If bd_focus is none, prefer yoke claim.
  3) If reviewer_agent_status is missing, use manual yoke review flags.

Example:
  yoke status
`)
}

func printDaemonUsage() {
	fmt.Print(`Usage:
  yoke daemon [options]

Purpose:
  Run an automatic code -> review loop for bd issues using configured writer/reviewer commands.

Loop priority (each iteration):
  1) Review first issue from the review queue (status blocked + label yoke:in_review).
  2) Otherwise run writer command on focused/in_progress issue.
  3) Otherwise claim next ready open issue from bd.
  4) Otherwise idle (sleep and poll again in continuous mode).
  5) If max iterations are reached without consensus, daemon notifies and leaves PR draft/open.

Command contract:
  - Writer command comes from YOKE_WRITER_CMD (or --writer-cmd override).
  - Reviewer command comes from YOKE_REVIEW_CMD (or --reviewer-cmd override).
  - Both run with env vars:
      ISSUE_ID, ROOT_DIR, BD_PREFIX, YOKE_ROLE
  - Commands must transition bd workflow state (writer -> submit/review queue, reviewer -> close or in_progress).
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
  yoke claim [<prefix>-issue-id] [options]

Purpose:
  Move an issue into active work and place the repository on the matching branch.

Behavior:
  - If issue id omitted, picks first issue from bd open+ready list.
  - If issue id is an epic, runs an epic improvement cycle (writer/reviewer alternating) before task claim.
  - Improvement cycle pass count defaults to 5 and can be limited with --improvement-passes.
  - If improvement is already marked complete but clarification tasks have comments, yoke reruns improvement automatically.
  - Clarification tasks with comments are auto-closed before selecting the next child task.
  - In-progress child tasks with unmet blocking dependencies are skipped.
  - Epic improvement reports are saved in .yoke/epic-improvement-reports/<epic-id>/.
  - If issue id is an epic, claims the next ready/in-progress child task in that epic.
  - If an epic has no remaining open child tasks, yoke closes the epic and exits.
  - Runs bd update <issue> --status in_progress.
  - Removes yoke review-queue label if present.
  - Switches to existing branch yoke/<issue> or creates it.

Inputs:
  issue-id    Optional. Explicit issue id (example uses prefix from YOKE_BD_PREFIX).

Options:
  --improvement-passes N   Limit epic improvement passes (1-5, default 5).

Examples:
  yoke claim
  yoke claim bd-a1b2
  yoke claim bd-a1b2 --improvement-passes 2

Side effects:
  - bd status transition to in_progress
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
  2) Writes a handoff comment to the bd issue.
  3) Moves issue into review queue (status blocked + label yoke:in_review).
  4) Pushes branch and opens draft PR when configured tools are available.
  5) Posts writer handoff summary comment to the branch PR.

Inputs:
  issue-id    Optional. If omitted, inferred from current branch name using YOKE_BD_PREFIX.

Options:
  --done TEXT          Required. What is complete now.
  --remaining TEXT     Required. What remains.
  --decision TEXT      Optional. Key decision made.
  --uncertain TEXT     Optional. Open uncertainty.
  --checks CMD         Optional. Override check command/script.
  --no-push            Do not push branch.
  --no-pr              Do not create or update PR.
  --no-pr-comment      Do not post writer handoff comment to PR.

Examples:
  yoke submit bd-a1b2 --done "Added auth flow" --remaining "Add tests"
  yoke submit --done "Refactor complete" --remaining "None" --no-pr
`)
}

func printReviewUsage() {
	fmt.Print(`Usage:
  yoke review [<prefix>-issue-id] [options]

Purpose:
  Execute reviewer step and finalize review outcome for a bd issue.

Behavior:
  - If issue id omitted, selects first issue in review queue (blocked + yoke:in_review).
  - Optional reviewer automation can run before final action.
  - Reviewer automation receives ISSUE_ID, ROOT_DIR, BD_PREFIX, and YOKE_ROLE=reviewer.
  - Approve closes review path and marks the issue PR ready for review (lifts draft).
  - Reject adds a rejection note and returns work to writer path (in_progress, removes yoke:in_review).
  - Approve/reject/note actions post reviewer update comments to the branch PR.

Inputs:
  issue-id    Optional. Explicit issue id using YOKE_BD_PREFIX.

Options:
  --agent              Run YOKE_REVIEW_CMD before final action.
  --note TEXT          Add reviewer note to bd issue.
  --approve            Approve issue (bd close).
  --reject TEXT        Reject issue with reason.
  --no-pr-comment      Do not post reviewer update comment to PR.

Examples:
  yoke review bd-a1b2 --agent --approve
  yoke review bd-a1b2 --reject "Missing edge-case test coverage"
  yoke review --note "Verified behavior locally"
`)
}
