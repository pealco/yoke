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

	if got := extractIssueID("next: td-a1b2 ready", "td"); got != "td-a1b2" {
		t.Fatalf("expected td-a1b2, got %q", got)
	}
	if got := extractIssueID("next: work-a1b2 ready", "work"); got != "work-a1b2" {
		t.Fatalf("expected work-a1b2, got %q", got)
	}

	if got := extractIssueID("no issue here", "td"); got != "" {
		t.Fatalf("expected empty issue ID, got %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.sh")

	content := `# shellcheck shell=bash
YOKE_BASE_BRANCH="develop"
YOKE_CHECK_CMD=".yoke/checks.sh"
YOKE_TD_PREFIX="work"
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
	if cfg.TDPrefix != "work" {
		t.Fatalf("TDPrefix = %q", cfg.TDPrefix)
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

	got := branchForIssue("td-abc123")
	if got != "yoke/td-abc123" {
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

func TestNormalizeTDPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "td", want: "td", ok: true},
		{input: "WORK", want: "work", ok: true},
		{input: "team_1", want: "team_1", ok: true},
		{input: "bad-", want: "", ok: false},
		{input: "a b", want: "", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeTDPrefix(tc.input)
			if tc.ok && err != nil {
				t.Fatalf("normalizeTDPrefix(%q) unexpected error: %v", tc.input, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("normalizeTDPrefix(%q) expected error", tc.input)
			}
			if got != tc.want {
				t.Fatalf("normalizeTDPrefix(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestLooksLikeIssueID(t *testing.T) {
	t.Parallel()

	if !looksLikeIssueID("work-a1b2", "work") {
		t.Fatalf("expected issue ID to match configured prefix")
	}
	if looksLikeIssueID("td-a1b2", "work") {
		t.Fatalf("did not expect mismatched prefix to match")
	}
}

func TestIssueOrNone(t *testing.T) {
	t.Parallel()

	if got := issueOrNone("td-a1b2"); got != "td-a1b2" {
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

func TestParseTDListIssuesJSON(t *testing.T) {
	t.Parallel()

	raw := `[
  {"id":"td-a1","status":"in_progress"},
  {"id":"td-b2","status":"in_review"}
]`
	issues, err := parseTDListIssuesJSON(raw)
	if err != nil {
		t.Fatalf("parseTDListIssuesJSON error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].ID != "td-a1" || issues[1].Status != "in_review" {
		t.Fatalf("unexpected issues payload: %#v", issues)
	}
}

func TestFirstMatchingIssueID(t *testing.T) {
	t.Parallel()

	issues := []tdListIssue{
		{ID: "work-a1", Status: "in_progress"},
		{ID: "work-b2", Status: "in_review"},
	}
	if got := firstMatchingIssueID(issues, "work", "in_progress"); got != "work-a1" {
		t.Fatalf("firstMatchingIssueID in_progress = %q", got)
	}
	if got := firstMatchingIssueID(issues, "work", "in_review"); got != "work-b2" {
		t.Fatalf("firstMatchingIssueID in_review = %q", got)
	}
	if got := firstMatchingIssueID(issues, "td", "in_progress"); got != "" {
		t.Fatalf("firstMatchingIssueID mismatched prefix = %q", got)
	}
}

func TestParseIssueStatusJSON(t *testing.T) {
	t.Parallel()

	if got, err := parseIssueStatusJSON(`{"id":"td-a1","status":"in_review"}`); err != nil || got != "in_review" {
		t.Fatalf("parseIssueStatusJSON valid = (%q, %v)", got, err)
	}
	if _, err := parseIssueStatusJSON(`{"id":"td-a1"}`); err == nil {
		t.Fatalf("parseIssueStatusJSON missing status expected error")
	}
}

func TestParseFocusedIssueID(t *testing.T) {
	t.Parallel()

	currentOutput := `SESSION: ses_123
FOCUSED: work-a1 Implement daemon
REVIEWS (1): work-b2 In review
`
	if got := parseFocusedIssueID(currentOutput, "work"); got != "work-a1" {
		t.Fatalf("parseFocusedIssueID = %q, want work-a1", got)
	}
	if got := parseFocusedIssueID("No active work", "work"); got != "" {
		t.Fatalf("parseFocusedIssueID without focused = %q, want empty", got)
	}
}

func TestParseOpenPRFromListJSON(t *testing.T) {
	t.Parallel()

	number, url, ok := parseOpenPRFromListJSON(`[{"number":42,"url":"https://example.com/pr/42"}]`)
	if !ok {
		t.Fatalf("expected PR parse to succeed")
	}
	if number != "42" {
		t.Fatalf("number = %q", number)
	}
	if url != "https://example.com/pr/42" {
		t.Fatalf("url = %q", url)
	}

	if _, _, ok := parseOpenPRFromListJSON(`[]`); ok {
		t.Fatalf("expected empty list to return no PR")
	}
	if _, _, ok := parseOpenPRFromListJSON(`not-json`); ok {
		t.Fatalf("expected invalid JSON to return no PR")
	}
}

func TestFormatWriterPRComment(t *testing.T) {
	t.Parallel()

	comment := formatWriterPRComment("td-a1b2", "done text", "remaining text", "decision text", "uncertain text", "make check")
	if !contains(comment, "## Writer -> Reviewer Handoff") {
		t.Fatalf("missing handoff heading: %s", comment)
	}
	if !contains(comment, "- Issue: `td-a1b2`") {
		t.Fatalf("missing issue line: %s", comment)
	}
	if !contains(comment, "- Checks: `make check` passed") {
		t.Fatalf("missing checks line: %s", comment)
	}
}

func TestFormatReviewerPRComment(t *testing.T) {
	t.Parallel()

	comment := formatReviewerPRComment("td-a1b2", "reject", "needs tests", "note text", true)
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
