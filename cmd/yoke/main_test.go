package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseShellValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		out  string
	}{
		{name: "double quoted", in: `"main"`, out: "main"},
		{name: "single quoted", in: `'main'`, out: "main"},
		{name: "trim comment", in: `main # comment`, out: "main"},
		{name: "keep hash in quote", in: `"main # value"`, out: "main # value"},
		{name: "raw value", in: `  value  `, out: "value"},
		{name: "empty", in: ``, out: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseShellValue(tc.in)
			if got != tc.out {
				t.Fatalf("parseShellValue(%q) = %q, want %q", tc.in, got, tc.out)
			}
		})
	}
}

func TestExtractIssueID(t *testing.T) {
	t.Parallel()

	if got := extractIssueID("next: bd-a1b2 ready", "bd"); got != "bd-a1b2" {
		t.Fatalf("expected bd-a1b2, got %q", got)
	}
	if got := extractIssueID("next: bd-a1b2.3 ready", "bd"); got != "bd-a1b2.3" {
		t.Fatalf("expected bd-a1b2.3, got %q", got)
	}
	if got := extractIssueID("next: work-a1b2 ready", "work"); got != "work-a1b2" {
		t.Fatalf("expected work-a1b2, got %q", got)
	}

	if got := extractIssueID("no issue here", "bd"); got != "" {
		t.Fatalf("expected empty issue ID, got %q", got)
	}
}

func TestExtractIssueIDAnyPrefix(t *testing.T) {
	t.Parallel()

	if got := extractIssueIDAnyPrefix("working on yoke-3kg.1 next"); got != "yoke-3kg.1" {
		t.Fatalf("expected yoke-3kg.1, got %q", got)
	}
	if got := extractIssueIDAnyPrefix("no issue here"); got != "" {
		t.Fatalf("expected empty issue ID, got %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.sh")

	content := `# shellcheck shell=bash
YOKE_BASE_BRANCH="develop"
YOKE_CHECK_CMD=".yoke/checks.sh"
YOKE_BD_PREFIX="work"
YOKE_WRITER_AGENT="codex"
YOKE_WRITER_CMD='echo writing'
YOKE_REVIEWER_AGENT="claude"
YOKE_REVIEW_CMD='echo reviewing'
YOKE_PR_TEMPLATE=".github/pull_request_template.md"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("YOKE_CONFIG", cfgPath)
	cfg, err := loadConfig(tmp)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.BaseBranch != "develop" {
		t.Fatalf("BaseBranch = %q, want develop", cfg.BaseBranch)
	}
	if cfg.CheckCmd != ".yoke/checks.sh" {
		t.Fatalf("CheckCmd = %q", cfg.CheckCmd)
	}
	if cfg.BDPrefix != "work" {
		t.Fatalf("BDPrefix = %q", cfg.BDPrefix)
	}
	if cfg.WriterAgent != "codex" {
		t.Fatalf("WriterAgent = %q", cfg.WriterAgent)
	}
	if cfg.WriterCmd != "echo writing" {
		t.Fatalf("WriterCmd = %q", cfg.WriterCmd)
	}
	if cfg.ReviewerAgent != "claude" {
		t.Fatalf("ReviewerAgent = %q", cfg.ReviewerAgent)
	}
	if cfg.ReviewCmd != "echo reviewing" {
		t.Fatalf("ReviewCmd = %q", cfg.ReviewCmd)
	}
	if cfg.PRTemplate != ".github/pull_request_template.md" {
		t.Fatalf("PRTemplate = %q", cfg.PRTemplate)
	}
}

func TestBranchForIssue(t *testing.T) {
	t.Parallel()

	got := branchForIssue("bd-abc123")
	if got != "yoke/bd-abc123" {
		t.Fatalf("branchForIssue returned %q", got)
	}
}

func TestWorktreePathForIssue(t *testing.T) {
	t.Parallel()

	root := filepath.Join(string(filepath.Separator), "tmp", "repo")
	got := worktreePathForIssue(root, "bd-abc123")
	want := filepath.Join(root, ".yoke", "worktrees", "bd-abc123")
	if got != want {
		t.Fatalf("worktreePathForIssue = %q, want %q", got, want)
	}
}

func TestDaemonFocusIssueLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if got := daemonFocusedIssue(root); got != "" {
		t.Fatalf("expected empty focus issue before write, got %q", got)
	}

	if err := writeDaemonFocusIssue(root, "YOKE-3KG.1"); err != nil {
		t.Fatalf("writeDaemonFocusIssue: %v", err)
	}
	if got := daemonFocusedIssue(root); got != "yoke-3kg.1" {
		t.Fatalf("daemonFocusedIssue = %q, want yoke-3kg.1", got)
	}

	clearDaemonFocusIssue(root)
	if got := daemonFocusedIssue(root); got != "" {
		t.Fatalf("expected empty focus issue after clear, got %q", got)
	}
}

func TestParseGitWorktreeListPorcelain(t *testing.T) {
	t.Parallel()

	raw := `worktree /tmp/repo
HEAD 1234567890
branch refs/heads/main

worktree /tmp/repo/.yoke/worktrees/bd-a1
HEAD abcdef0123
branch refs/heads/yoke/bd-a1
`
	got := parseGitWorktreeListPorcelain(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 worktree paths, got %d", len(got))
	}
	if got[0] != "/tmp/repo" {
		t.Fatalf("first worktree path = %q", got[0])
	}
	if got[1] != "/tmp/repo/.yoke/worktrees/bd-a1" {
		t.Fatalf("second worktree path = %q", got[1])
	}
}

func TestParseGitWorktreeListEntries(t *testing.T) {
	t.Parallel()

	raw := `worktree /tmp/repo
HEAD 1234567890
branch refs/heads/main

worktree /tmp/repo/.yoke/worktrees/bd-a1
HEAD abcdef0123
branch refs/heads/yoke/bd-a1
`
	got := parseGitWorktreeListEntries(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 worktree entries, got %d", len(got))
	}
	if got[0].Path != "/tmp/repo" || got[0].Branch != "main" {
		t.Fatalf("unexpected first entry: %#v", got[0])
	}
	if got[1].Path != "/tmp/repo/.yoke/worktrees/bd-a1" || got[1].Branch != "yoke/bd-a1" {
		t.Fatalf("unexpected second entry: %#v", got[1])
	}
}

func TestNormalizeAgentID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "codex", want: "codex", ok: true},
		{input: "claude", want: "claude", ok: true},
		{input: "claude-code", want: "claude", ok: true},
		{input: "unknown", want: "", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, ok := normalizeAgentID(tc.input)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("normalizeAgentID(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDetectAvailableAgents(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})

	lookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/local/bin/codex", nil
		case "claude":
			return "/usr/local/bin/claude", nil
		default:
			return "", os.ErrNotExist
		}
	}

	available := detectAvailableAgents()
	if len(available) != 2 {
		t.Fatalf("expected 2 detected agents, got %d", len(available))
	}

	if available[0].ID != "codex" {
		t.Fatalf("first agent = %q, want codex", available[0].ID)
	}
	if available[1].ID != "claude" {
		t.Fatalf("second agent = %q, want claude", available[1].ID)
	}
}

func TestNormalizeBDPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "bd", want: "bd", ok: true},
		{input: "WORK", want: "work", ok: true},
		{input: "team_1", want: "team_1", ok: true},
		{input: "repo.name", want: "repo.name", ok: true},
		{input: "bad-", want: "", ok: false},
		{input: "a b", want: "", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeBDPrefix(tc.input)
			if tc.ok && err != nil {
				t.Fatalf("normalizeBDPrefix(%q) unexpected error: %v", tc.input, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("normalizeBDPrefix(%q) expected error", tc.input)
			}
			if got != tc.want {
				t.Fatalf("normalizeBDPrefix(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestLooksLikeIssueID(t *testing.T) {
	t.Parallel()

	if !looksLikeIssueID("work-a1b2", "work") {
		t.Fatalf("expected issue ID to match configured prefix")
	}
	if looksLikeIssueID("bd-a1b2", "work") {
		t.Fatalf("did not expect mismatched prefix to match")
	}
}

func TestLooksLikeIssueIDAnyPrefix(t *testing.T) {
	t.Parallel()

	if !looksLikeIssueIDAnyPrefix("yoke-3kg.1") {
		t.Fatalf("expected yoke-3kg.1 to match issue pattern")
	}
	if !looksLikeIssueIDAnyPrefix("bd-a1b2") {
		t.Fatalf("expected bd-a1b2 to match issue pattern")
	}
	if looksLikeIssueIDAnyPrefix("plaintext") {
		t.Fatalf("did not expect non-issue value to match issue pattern")
	}
}

func TestIssueOrNone(t *testing.T) {
	t.Parallel()

	if got := issueOrNone("bd-a1b2"); got != "bd-a1b2" {
		t.Fatalf("issueOrNone returned %q", got)
	}
	if got := issueOrNone(""); got != "none" {
		t.Fatalf("issueOrNone empty = %q, want none", got)
	}
}

func TestAvailabilityLabel(t *testing.T) {
	t.Parallel()

	if got := availabilityLabel(true); got != "available" {
		t.Fatalf("availabilityLabel(true) = %q", got)
	}
	if got := availabilityLabel(false); got != "missing" {
		t.Fatalf("availabilityLabel(false) = %q", got)
	}
}

func TestConfiguredAgentStatus(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})

	lookPath = func(file string) (string, error) {
		if file == "codex" {
			return "/usr/local/bin/codex", nil
		}
		return "", os.ErrNotExist
	}

	if got := configuredAgentStatus(""); got != "unset" {
		t.Fatalf("configuredAgentStatus(\"\") = %q", got)
	}
	if got := configuredAgentStatus("codex"); got != "available via codex" {
		t.Fatalf("configuredAgentStatus(codex) = %q", got)
	}
}

func TestCommandConfigStatus(t *testing.T) {
	t.Parallel()

	if got := commandConfigStatus(""); got != "unset" {
		t.Fatalf("commandConfigStatus(\"\") = %q", got)
	}
	if got := commandConfigStatus("echo hi"); got != "configured" {
		t.Fatalf("commandConfigStatus configured = %q", got)
	}
}

func TestParseDaemonInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{input: "30", want: 30 * time.Second},
		{input: "45s", want: 45 * time.Second},
		{input: "2m", want: 2 * time.Minute},
		{input: "0", wantErr: true},
		{input: "bad", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := parseDaemonInterval(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("parseDaemonInterval(%q) expected error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("parseDaemonInterval(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseDaemonInterval(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseClaimArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		wantIssue string
		wantPass  int
		wantErr   string
	}{
		{
			name:      "defaults",
			args:      nil,
			wantIssue: "",
			wantPass:  epicPassCount,
		},
		{
			name:      "issue only",
			args:      []string{"bd-a1b2"},
			wantIssue: "bd-a1b2",
			wantPass:  epicPassCount,
		},
		{
			name:      "limited passes",
			args:      []string{"bd-a1b2", "--improvement-passes", "2"},
			wantIssue: "bd-a1b2",
			wantPass:  2,
		},
		{
			name:      "limited passes without issue",
			args:      []string{"--improvement-passes", "3"},
			wantIssue: "",
			wantPass:  3,
		},
		{
			name:      "skip passes",
			args:      []string{"--improvement-passes", "0"},
			wantIssue: "",
			wantPass:  0,
		},
		{
			name:    "missing pass value",
			args:    []string{"--improvement-passes"},
			wantErr: "--improvement-passes requires a value",
		},
		{
			name:    "pass value out of range low",
			args:    []string{"--improvement-passes", "-1"},
			wantErr: "--improvement-passes must be an integer between 0 and 5",
		},
		{
			name:    "pass value out of range high",
			args:    []string{"--improvement-passes", "6"},
			wantErr: "--improvement-passes must be an integer between 0 and 5",
		},
		{
			name:    "unknown flag",
			args:    []string{"--unknown"},
			wantErr: "unknown claim argument: --unknown",
		},
		{
			name:    "too many positionals",
			args:    []string{"bd-a1", "bd-a2"},
			wantErr: "usage: yoke claim [<prefix>-issue-id] [--improvement-passes N]",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotIssue, gotPass, err := parseClaimArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseClaimArgs(%v) expected error %q", tc.args, tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("parseClaimArgs(%v) error = %q, want %q", tc.args, err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseClaimArgs(%v) unexpected error: %v", tc.args, err)
			}
			if gotIssue != tc.wantIssue {
				t.Fatalf("parseClaimArgs(%v) issue = %q, want %q", tc.args, gotIssue, tc.wantIssue)
			}
			if gotPass != tc.wantPass {
				t.Fatalf("parseClaimArgs(%v) pass limit = %d, want %d", tc.args, gotPass, tc.wantPass)
			}
		})
	}
}

func TestRunEpicImprovementCycleSkipWhenPassLimitZero(t *testing.T) {
	t.Parallel()

	if err := runEpicImprovementCycle(t.TempDir(), config{}, bdListIssue{ID: "bd-a1b2", IssueType: "epic"}, 0); err != nil {
		t.Fatalf("runEpicImprovementCycle passLimit=0 unexpected error: %v", err)
	}
}

func TestParseBDListIssuesJSON(t *testing.T) {
	t.Parallel()

	raw := `[
  {"id":"bd-a1","status":"in_progress"},
  {"id":"bd-b2","status":"blocked","labels":["yoke:in_review"]}
]`
	issues, err := parseBDListIssuesJSON(raw)
	if err != nil {
		t.Fatalf("parseBDListIssuesJSON error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].ID != "bd-a1" || issues[1].Status != "blocked" {
		t.Fatalf("unexpected issues payload: %#v", issues)
	}
}

func TestParseBDCommentsJSON(t *testing.T) {
	t.Parallel()

	raw := `[
  {"id":1,"issue_id":"bd-a1","author":"Pedro","text":"Answer text","created_at":"2026-01-01T00:00:00Z"}
]`
	comments, err := parseBDCommentsJSON(raw)
	if err != nil {
		t.Fatalf("parseBDCommentsJSON error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].IssueID != "bd-a1" || comments[0].Text != "Answer text" {
		t.Fatalf("unexpected comments payload: %#v", comments)
	}
}

func TestParseBDDependencyEdgesJSON(t *testing.T) {
	t.Parallel()

	edgeListRaw := `[
		{"issue_id":"bd-a1","depends_on_id":"bd-a2","type":"blocks"},
		{"issue_id":"bd-a1","depends_on_id":"bd-a3","type":"parent-child"}
	]`
	edges, err := parseBDDependencyEdgesJSON(edgeListRaw)
	if err != nil {
		t.Fatalf("parseBDDependencyEdgesJSON edge list error: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges from edge list, got %d", len(edges))
	}
	if edges[0].IssueID != "bd-a1" || edges[0].DependsOnID != "bd-a2" || edges[0].Type != "blocks" {
		t.Fatalf("unexpected first edge payload: %#v", edges[0])
	}

	issueListRaw := `[
		{
			"id":"bd-a1",
			"dependencies":[
				{"depends_on_id":"bd-a2","type":"blocks"},
				{"depends_on_id":"bd-a3","type":"parent-child"}
			]
		}
	]`
	edges, err = parseBDDependencyEdgesJSON(issueListRaw)
	if err != nil {
		t.Fatalf("parseBDDependencyEdgesJSON issue payload error: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges from issue payload, got %d", len(edges))
	}
	if edges[0].IssueID != "bd-a1" || edges[0].DependsOnID != "bd-a2" || edges[0].Type != "blocks" {
		t.Fatalf("unexpected first issue-derived edge payload: %#v", edges[0])
	}
}

func TestIsClarificationNeededTitle(t *testing.T) {
	t.Parallel()

	if !isClarificationNeededTitle("Clarification needed: intake contract") {
		t.Fatalf("expected title to match clarification prefix")
	}
	if !isClarificationNeededTitle("  clarification needed: scope  ") {
		t.Fatalf("expected case-insensitive clarification prefix match")
	}
	if isClarificationNeededTitle("Follow-up: intake contract") {
		t.Fatalf("did not expect non-clarification title to match")
	}
}

func TestClarificationTaskReadyForAutoClose(t *testing.T) {
	t.Parallel()

	if !clarificationTaskReadyForAutoClose(bdListIssue{
		Title:        "Clarification needed: input behavior",
		Status:       "open",
		CommentCount: 1,
	}) {
		t.Fatalf("expected open clarification with comments to be auto-closable")
	}

	if clarificationTaskReadyForAutoClose(bdListIssue{
		Title:        "Clarification needed: input behavior",
		Status:       "closed",
		CommentCount: 1,
	}) {
		t.Fatalf("did not expect closed clarification to be auto-closable")
	}

	if clarificationTaskReadyForAutoClose(bdListIssue{
		Title:        "Clarification needed: input behavior",
		Status:       "open",
		CommentCount: 0,
	}) {
		t.Fatalf("did not expect clarification without comments to be auto-closable")
	}

	if clarificationTaskReadyForAutoClose(bdListIssue{
		Title:        "Task: implement intake",
		Status:       "open",
		CommentCount: 2,
	}) {
		t.Fatalf("did not expect non-clarification task to be auto-closable")
	}
}

func TestHasOpenBlockingDependencies(t *testing.T) {
	t.Parallel()

	if !hasOpenBlockingDependencies([]bdListIssue{
		{ID: "bd-a1", DependencyType: "blocks", Status: "open"},
	}) {
		t.Fatalf("expected open blocks dependency to be considered unmet")
	}

	if hasOpenBlockingDependencies([]bdListIssue{
		{ID: "bd-a1", DependencyType: "parent-child", Status: "open"},
	}) {
		t.Fatalf("did not expect parent-child dependency to be treated as blocker")
	}

	if hasOpenBlockingDependencies([]bdListIssue{
		{ID: "bd-a1", DependencyType: "blocks", Status: "closed"},
		{ID: "bd-a2", DependencyType: "blocks", Status: "closed"},
	}) {
		t.Fatalf("did not expect all-closed blockers to be considered unmet")
	}

	if !hasOpenBlockingDependencies([]bdListIssue{
		{ID: "bd-a1", DependencyType: "blocks", Status: "blocked", Labels: []string{reviewQueueLabel}},
	}) {
		t.Fatalf("expected in-review blocker dependency to be considered unmet")
	}
}

func TestHasDependencyTypeEntries(t *testing.T) {
	t.Parallel()

	if hasDependencyTypeEntries([]bdListIssue{
		{ID: "bd-a1", Status: "open"},
	}) {
		t.Fatalf("did not expect dependency-type detection without dependency_type values")
	}

	if !hasDependencyTypeEntries([]bdListIssue{
		{ID: "bd-a1", Status: "open", DependencyType: "blocks"},
	}) {
		t.Fatalf("expected dependency-type detection when dependency_type is present")
	}
}

func TestHasOpenBlockingDependencyEdges(t *testing.T) {
	t.Parallel()

	statuses := map[string]string{
		"bd-a2": "open",
		"bd-a3": "closed",
	}
	lookupCalls := 0
	statusLookup := func(issueID string) (string, error) {
		lookupCalls++
		status, ok := statuses[issueID]
		if !ok {
			return "", errors.New("missing issue status")
		}
		return status, nil
	}

	hasOpen, err := hasOpenBlockingDependencyEdges("bd-a1", []bdDependencyEdge{
		{IssueID: "bd-a1", DependsOnID: "bd-a3", Type: "blocks"},
		{IssueID: "bd-a1", DependsOnID: "bd-a2", Type: "blocks"},
		{IssueID: "bd-a1", DependsOnID: "bd-a4", Type: "parent-child"},
	}, statusLookup)
	if err != nil {
		t.Fatalf("hasOpenBlockingDependencyEdges unexpected error: %v", err)
	}
	if !hasOpen {
		t.Fatalf("expected open blocking dependency to be detected")
	}
	if lookupCalls != 2 {
		t.Fatalf("expected 2 status lookups, got %d", lookupCalls)
	}

	hasOpen, err = hasOpenBlockingDependencyEdges("bd-a1", []bdDependencyEdge{
		{IssueID: "bd-a1", DependsOnID: "bd-a3", Type: "blocks"},
	}, statusLookup)
	if err != nil {
		t.Fatalf("hasOpenBlockingDependencyEdges all-closed unexpected error: %v", err)
	}
	if hasOpen {
		t.Fatalf("did not expect closed blockers to be considered open")
	}
}

func TestFilterClaimCandidatesForEpic(t *testing.T) {
	t.Parallel()

	workItemIDs := map[string]struct{}{
		"epic.1": {},
		"epic.2": {},
	}
	openDeps := map[string]bool{
		"epic.2": true,
	}
	filtered, skippedBlocked, ignoredOutsideEpic, err := filterClaimCandidatesForEpic([]bdListIssue{
		{ID: "epic"},
		{ID: "epic.1"},
		{ID: "epic.2"},
	}, workItemIDs, func(issueID string) (bool, error) {
		return openDeps[issueID], nil
	})
	if err != nil {
		t.Fatalf("filterClaimCandidatesForEpic unexpected error: %v", err)
	}
	if ignoredOutsideEpic != 1 {
		t.Fatalf("expected 1 outside-epic candidate, got %d", ignoredOutsideEpic)
	}
	if len(skippedBlocked) != 1 || skippedBlocked[0] != "epic.2" {
		t.Fatalf("unexpected skipped blocked list: %#v", skippedBlocked)
	}
	if len(filtered) != 1 || filtered[0].ID != "epic.1" {
		t.Fatalf("unexpected filtered candidates: %#v", filtered)
	}
}

func TestFirstMatchingIssueID(t *testing.T) {
	t.Parallel()

	issues := []bdListIssue{
		{ID: "work-a1", Status: "in_progress"},
		{ID: "work-b2", Status: "blocked", Labels: []string{reviewQueueLabel}},
	}
	if got := firstMatchingIssueID(issues, "work", "in_progress"); got != "work-a1" {
		t.Fatalf("firstMatchingIssueID in_progress = %q", got)
	}
	if got := firstMatchingIssueID(issues, "work", "in_review"); got != "work-b2" {
		t.Fatalf("firstMatchingIssueID in_review = %q", got)
	}
	if got := firstMatchingIssueID(issues, "bd", "in_progress"); got != "" {
		t.Fatalf("firstMatchingIssueID mismatched prefix = %q", got)
	}
}

func TestParseIssueStatusJSON(t *testing.T) {
	t.Parallel()

	if got, err := parseIssueStatusJSON(`[{"id":"bd-a1","status":"blocked","labels":["yoke:in_review"]}]`); err != nil || got != "in_review" {
		t.Fatalf("parseIssueStatusJSON valid = (%q, %v)", got, err)
	}
	if got, err := parseIssueStatusJSON(`[{"id":"bd-a1","status":"closed"}]`); err != nil || got != "closed" {
		t.Fatalf("parseIssueStatusJSON closed = (%q, %v)", got, err)
	}
	if _, err := parseIssueStatusJSON(`[{"id":"bd-a1"}]`); err == nil {
		t.Fatalf("parseIssueStatusJSON missing status expected error")
	}
}

func TestParseOpenPRFromListJSON(t *testing.T) {
	t.Parallel()

	number, url, isDraft, ok := parseOpenPRFromListJSON(`[{"number":42,"url":"https://example.com/pr/42","isDraft":true}]`)
	if !ok {
		t.Fatalf("expected PR parse to succeed")
	}
	if number != "42" {
		t.Fatalf("number = %q", number)
	}
	if url != "https://example.com/pr/42" {
		t.Fatalf("url = %q", url)
	}
	if !isDraft {
		t.Fatalf("expected isDraft=true")
	}

	if _, _, _, ok := parseOpenPRFromListJSON(`[]`); ok {
		t.Fatalf("expected empty list to return no PR")
	}
	if _, _, _, ok := parseOpenPRFromListJSON(`not-json`); ok {
		t.Fatalf("expected invalid JSON to return no PR")
	}
}

func TestFormatWriterPRComment(t *testing.T) {
	t.Parallel()

	comment := formatWriterPRComment("bd-a1b2", "done text", "remaining text", "decision text", "uncertain text", "make check")
	if !contains(comment, "## Writer -> Reviewer Handoff") {
		t.Fatalf("missing handoff heading: %s", comment)
	}
	if !contains(comment, "- Issue: `bd-a1b2`") {
		t.Fatalf("missing issue line: %s", comment)
	}
	if !contains(comment, "- Checks: `make check` passed") {
		t.Fatalf("missing checks line: %s", comment)
	}
}

func TestFormatIssueHandoffComment(t *testing.T) {
	t.Parallel()

	comment := formatIssueHandoffComment("done text", "remaining text", "decision text", "uncertain text", "make check")
	if !contains(comment, "Writer handoff:") {
		t.Fatalf("missing handoff heading: %s", comment)
	}
	if !contains(comment, "- Checks: `make check` passed") {
		t.Fatalf("missing checks line: %s", comment)
	}
}

func TestFormatReviewerPRComment(t *testing.T) {
	t.Parallel()

	comment := formatReviewerPRComment("bd-a1b2", "reject", "needs tests", "note text", true)
	if !contains(comment, "## Reviewer Update") {
		t.Fatalf("missing reviewer heading: %s", comment)
	}
	if !contains(comment, "- Decision: reject") {
		t.Fatalf("missing decision line: %s", comment)
	}
	if !contains(comment, "- Reject reason: needs tests") {
		t.Fatalf("missing reject reason line: %s", comment)
	}
	if !contains(comment, "- Reviewer command: executed") {
		t.Fatalf("missing reviewer command marker: %s", comment)
	}
}

func TestFormatDaemonNoConsensusPRComment(t *testing.T) {
	t.Parallel()

	comment := formatDaemonNoConsensusPRComment("bd-a1b2", "in_review", 10)
	if !contains(comment, "## Daemon Notice") {
		t.Fatalf("missing daemon heading: %s", comment)
	}
	if !contains(comment, "- PR state: left in draft for manual intervention") {
		t.Fatalf("missing draft note: %s", comment)
	}
}

func TestPickEpicChildToClaimPrefersInProgress(t *testing.T) {
	t.Parallel()

	descendants := []bdListIssue{
		{ID: "bd-epic.1", IssueType: "task", Status: "open"},
		{ID: "bd-epic.2", IssueType: "task", Status: "open"},
	}
	inProgress := []bdListIssue{
		{ID: "bd-epic.2", IssueType: "task", Status: "in_progress"},
	}
	ready := []bdListIssue{
		{ID: "bd-epic.1", IssueType: "task", Status: "open"},
	}

	got, done := pickEpicChildToClaim(descendants, inProgress, ready)
	if got != "bd-epic.2" || done {
		t.Fatalf("pickEpicChildToClaim = (%q, %v), want (bd-epic.2, false)", got, done)
	}
}

func TestPickEpicChildToClaimReadyFallback(t *testing.T) {
	t.Parallel()

	descendants := []bdListIssue{
		{ID: "bd-epic.1", IssueType: "task", Status: "open"},
		{ID: "bd-epic.2", IssueType: "task", Status: "open"},
	}
	ready := []bdListIssue{
		{ID: "bd-epic.2", IssueType: "task", Status: "open"},
	}

	got, done := pickEpicChildToClaim(descendants, nil, ready)
	if got != "bd-epic.2" || done {
		t.Fatalf("pickEpicChildToClaim = (%q, %v), want (bd-epic.2, false)", got, done)
	}
}

func TestPickEpicChildToClaimComplete(t *testing.T) {
	t.Parallel()

	descendants := []bdListIssue{
		{ID: "bd-epic.1", IssueType: "task", Status: "closed"},
		{ID: "bd-epic.2", IssueType: "task", Status: "closed"},
		{ID: "bd-epic.3", IssueType: "epic", Status: "open"},
	}

	got, done := pickEpicChildToClaim(descendants, nil, nil)
	if got != "" || !done {
		t.Fatalf("pickEpicChildToClaim = (%q, %v), want (\"\", true)", got, done)
	}
}

func TestPickEpicChildToClaimBlocked(t *testing.T) {
	t.Parallel()

	descendants := []bdListIssue{
		{ID: "bd-epic.1", IssueType: "task", Status: "blocked"},
		{ID: "bd-epic.2", IssueType: "task", Status: "open"},
	}

	got, done := pickEpicChildToClaim(descendants, nil, nil)
	if got != "" || done {
		t.Fatalf("pickEpicChildToClaim = (%q, %v), want (\"\", false)", got, done)
	}
}

func TestRoleForPass(t *testing.T) {
	t.Parallel()

	if got := roleForPass(1); got != "writer" {
		t.Fatalf("roleForPass(1) = %q", got)
	}
	if got := roleForPass(2); got != "reviewer" {
		t.Fatalf("roleForPass(2) = %q", got)
	}
	if got := roleForPass(5); got != "writer" {
		t.Fatalf("roleForPass(5) = %q", got)
	}
}

func TestDaemonCommandWithExtraWritableDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "adds codex exec add-dir",
			input: `codex exec --full-auto --cd "$ROOT_DIR" "do work"`,
			want:  `codex exec --add-dir "$YOKE_MAIN_ROOT" --full-auto --cd "$ROOT_DIR" "do work"`,
		},
		{
			name:  "keeps existing add-dir",
			input: `codex exec --add-dir "/tmp" --full-auto "do work"`,
			want:  `codex exec --add-dir "/tmp" --full-auto "do work"`,
		},
		{
			name:  "non codex command unchanged",
			input: `echo "hello"`,
			want:  `echo "hello"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := daemonCommandWithExtraWritableDir(tc.input); got != tc.want {
				t.Fatalf("daemonCommandWithExtraWritableDir(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAppendOrPrependPath(t *testing.T) {
	t.Parallel()

	got := appendOrPrependPath([]string{"A=1", "PATH=/usr/bin"}, "/tmp/work/bin", "/tmp/main/bin")
	pathValue := ""
	for _, item := range got {
		if strings.HasPrefix(item, "PATH=") {
			pathValue = strings.TrimPrefix(item, "PATH=")
			break
		}
	}
	if pathValue == "" {
		t.Fatalf("PATH not found in env: %#v", got)
	}
	if !strings.HasPrefix(pathValue, "/tmp/work/bin"+string(os.PathListSeparator)+"/tmp/main/bin"+string(os.PathListSeparator)+"/usr/bin") {
		t.Fatalf("unexpected PATH value %q", pathValue)
	}
}

func TestDaemonLogFilterSuppressesRolloutNoise(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newDaemonLogFilterWriter(&out)
	_, err := w.Write([]byte("keep-one\n2026-02-17T08:00:14Z ERROR codex_core::rollout::list: state db missing rollout path for thread 123\nkeep-two\n"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "keep-one\n") || !strings.Contains(got, "keep-two\n") {
		t.Fatalf("expected non-noise lines to remain, got %q", got)
	}
	if strings.Contains(got, "state db missing rollout path for thread") {
		t.Fatalf("expected rollout noise to be suppressed, got %q", got)
	}
}

func TestDaemonLogFilterSuppressesMarkdownDiffFence(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newDaemonLogFilterWriter(&out)
	input := "before\n```diff\n-old\n+new\n```\nafter\n"
	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	got := out.String()
	if got != "before\nafter\n" {
		t.Fatalf("unexpected filtered output: %q", got)
	}
}

func TestDaemonLogFilterSuppressesRawGitDiff(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	w := newDaemonLogFilterWriter(&out)
	input := strings.Join([]string{
		"before",
		"diff --git a/file.txt b/file.txt",
		"index 1111111..2222222 100644",
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"after",
		"",
	}, "\n")
	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	got := out.String()
	if got != "before\nafter\n" {
		t.Fatalf("unexpected filtered output: %q", got)
	}
}

func TestBuildEpicImprovementPassPrompt(t *testing.T) {
	t.Parallel()

	prompt := buildEpicImprovementPassPrompt("bd-a1b2", 3, 5, "writer", nil)
	if !contains(prompt, "pass 3 of 5") {
		t.Fatalf("expected pass metadata in prompt: %s", prompt)
	}
	if !contains(prompt, "bd show bd-a1b2") {
		t.Fatalf("expected epic id replacement in prompt: %s", prompt)
	}
	if !contains(prompt, "No clarification-task comments were found.") {
		t.Fatalf("expected empty clarification marker in prompt: %s", prompt)
	}
}

func TestBuildEpicImprovementPassPromptWithClarifications(t *testing.T) {
	t.Parallel()

	prompt := buildEpicImprovementPassPrompt("bd-a1b2", 1, 2, "writer", []clarificationContext{
		{
			IssueID: "bd-a1b2.10",
			Title:   "Clarification needed: sample",
			Comments: []bdComment{
				{
					Author:    "Pedro",
					Text:      "Use stdin when piped input is present.",
					CreatedAt: "2026-02-17T05:53:27Z",
				},
			},
		},
	})

	if !contains(prompt, "Clarification needed: sample") {
		t.Fatalf("expected clarification title in prompt: %s", prompt)
	}
	if !contains(prompt, "Use stdin when piped input is present.") {
		t.Fatalf("expected clarification comment text in prompt: %s", prompt)
	}
}

func TestTruncateForPrompt(t *testing.T) {
	t.Parallel()

	if got := truncateForPrompt("abc", 10); got != "abc" {
		t.Fatalf("truncateForPrompt no-op = %q", got)
	}
	got := truncateForPrompt("abcdefghijklmnopqrstuvwxyz", 8)
	if !contains(got, "...[truncated]...") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}

func TestRunStatusHelp(t *testing.T) {
	t.Parallel()

	if err := run([]string{"status", "--help"}); err != nil {
		t.Fatalf("run status help: %v", err)
	}
}

func TestCmdHelpStatusTopic(t *testing.T) {
	t.Parallel()

	if err := cmdHelp([]string{"status"}); err != nil {
		t.Fatalf("cmdHelp status: %v", err)
	}
}

func TestRunDaemonHelp(t *testing.T) {
	t.Parallel()

	if err := run([]string{"daemon", "--help"}); err != nil {
		t.Fatalf("run daemon help: %v", err)
	}
}

func TestCmdHelpDaemonTopic(t *testing.T) {
	t.Parallel()

	if err := cmdHelp([]string{"daemon"}); err != nil {
		t.Fatalf("cmdHelp daemon: %v", err)
	}
}
