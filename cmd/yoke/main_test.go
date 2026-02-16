package main

import (
	"os"
	"path/filepath"
	"testing"
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
