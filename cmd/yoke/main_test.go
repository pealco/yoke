package main

import (
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
			name:    "missing pass value",
			args:    []string{"--improvement-passes"},
			wantErr: "--improvement-passes requires a value",
		},
		{
			name:    "pass value out of range low",
			args:    []string{"--improvement-passes", "0"},
			wantErr: "--improvement-passes must be an integer between 1 and 5",
		},
		{
			name:    "pass value out of range high",
			args:    []string{"--improvement-passes", "6"},
			wantErr: "--improvement-passes must be an integer between 1 and 5",
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
