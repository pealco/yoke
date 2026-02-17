package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateIntakePlanValid(t *testing.T) {
	t.Parallel()

	plan := intakePlan{
		Epic: intakePlanEpic{
			Title:       "Add intake command",
			Description: "Convert idea text into epic and tasks",
			Priority:    "high",
		},
		Tasks: []intakePlanTask{
			{
				Title:              "Define schema",
				Description:        "Create shared intake schema model",
				AcceptanceCriteria: []string{"Schema compiles", "Tests pass"},
			},
		},
	}

	if err := validateIntakePlan(plan); err != nil {
		t.Fatalf("validateIntakePlan(valid) unexpected error: %v", err)
	}
}

func TestValidateIntakePlanAllowsMoreThanOneTaskWithoutUpperBound(t *testing.T) {
	t.Parallel()

	tasks := make([]intakePlanTask, 0, 32)
	for i := 0; i < 32; i++ {
		tasks = append(tasks, intakePlanTask{
			Title:              fmt.Sprintf("Task %d", i+1),
			Description:        "Split work for a single agent",
			AcceptanceCriteria: []string{"Has a falsifiable outcome"},
		})
	}

	plan := intakePlan{
		Epic: intakePlanEpic{
			Title:       "Large decomposition",
			Description: "Allow judgment-based decomposition",
			Priority:    "medium",
		},
		Tasks: tasks,
	}

	if err := validateIntakePlan(plan); err != nil {
		t.Fatalf("validateIntakePlan(large-task-list) unexpected error: %v", err)
	}
}

func TestValidateIntakePlanRejectsMissingOrEmptyEpicFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		plan   intakePlan
		path   string
		reason string
	}{
		{
			name: "missing epic title",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Description: "Epic description",
					Priority:    "high",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.title",
			reason: "must be non-empty",
		},
		{
			name: "empty epic title",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Title:       "   ",
					Description: "Epic description",
					Priority:    "high",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.title",
			reason: "must be non-empty",
		},
		{
			name: "missing epic description",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Title:    "Epic title",
					Priority: "high",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.description",
			reason: "must be non-empty",
		},
		{
			name: "empty epic description",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Title:       "Epic title",
					Description: "   ",
					Priority:    "high",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.description",
			reason: "must be non-empty",
		},
		{
			name: "missing epic priority",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Title:       "Epic title",
					Description: "Epic description",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.priority",
			reason: "must be non-empty",
		},
		{
			name: "empty epic priority",
			plan: intakePlan{
				Epic: intakePlanEpic{
					Title:       "Epic title",
					Description: "Epic description",
					Priority:    "  ",
				},
				Tasks: []intakePlanTask{validTask()},
			},
			path:   "epic.priority",
			reason: "must be non-empty",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateIntakePlan(tc.plan)
			assertPlanValidationError(t, err, tc.path, tc.reason)
		})
	}
}

func TestValidateIntakePlanRejectsMissingOrEmptyTaskList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		tasks  []intakePlanTask
		reason string
	}{
		{
			name:   "missing task array",
			tasks:  nil,
			reason: "is required",
		},
		{
			name:   "empty task array",
			tasks:  []intakePlanTask{},
			reason: "must contain at least 1 task",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := intakePlan{
				Epic:  validEpic(),
				Tasks: tc.tasks,
			}
			err := validateIntakePlan(plan)
			assertPlanValidationError(t, err, "tasks", tc.reason)
		})
	}
}

func TestValidateIntakePlanRejectsMissingOrEmptyTaskFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		task   intakePlanTask
		path   string
		reason string
	}{
		{
			name: "missing task title",
			task: intakePlanTask{
				Description:        "Task description",
				AcceptanceCriteria: []string{"One criterion"},
			},
			path:   "tasks[0].title",
			reason: "must be non-empty",
		},
		{
			name: "empty task title",
			task: intakePlanTask{
				Title:              "   ",
				Description:        "Task description",
				AcceptanceCriteria: []string{"One criterion"},
			},
			path:   "tasks[0].title",
			reason: "must be non-empty",
		},
		{
			name: "missing task description",
			task: intakePlanTask{
				Title:              "Task title",
				AcceptanceCriteria: []string{"One criterion"},
			},
			path:   "tasks[0].description",
			reason: "must be non-empty",
		},
		{
			name: "empty task description",
			task: intakePlanTask{
				Title:              "Task title",
				Description:        "   ",
				AcceptanceCriteria: []string{"One criterion"},
			},
			path:   "tasks[0].description",
			reason: "must be non-empty",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := intakePlan{
				Epic:  validEpic(),
				Tasks: []intakePlanTask{tc.task},
			}
			err := validateIntakePlan(plan)
			assertPlanValidationError(t, err, tc.path, tc.reason)
		})
	}
}

func TestValidateIntakePlanRejectsMissingOrEmptyAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		task   intakePlanTask
		path   string
		reason string
	}{
		{
			name: "missing acceptance criteria",
			task: intakePlanTask{
				Title:       "Task title",
				Description: "Task description",
			},
			path:   "tasks[0].acceptance_criteria",
			reason: "is required",
		},
		{
			name: "empty acceptance criteria",
			task: intakePlanTask{
				Title:              "Task title",
				Description:        "Task description",
				AcceptanceCriteria: []string{},
			},
			path:   "tasks[0].acceptance_criteria",
			reason: "must contain at least 1 item",
		},
		{
			name: "empty acceptance criteria item",
			task: intakePlanTask{
				Title:              "Task title",
				Description:        "Task description",
				AcceptanceCriteria: []string{"First is good", "   "},
			},
			path:   "tasks[0].acceptance_criteria[1]",
			reason: "must be non-empty",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := intakePlan{
				Epic:  validEpic(),
				Tasks: []intakePlanTask{tc.task},
			}
			err := validateIntakePlan(plan)
			assertPlanValidationError(t, err, tc.path, tc.reason)
		})
	}
}

func TestParseGeneratedIntakePlanUsesSharedValidation(t *testing.T) {
	t.Parallel()

	raw := `{
  "epic": {
    "title": "",
    "description": "Build intake",
    "priority": "high"
  },
  "tasks": [
    {
      "title": "Task 1",
      "description": "Describe work",
      "acceptance_criteria": ["A criterion"]
    }
  ]
}`
	_, err := parseGeneratedIntakePlan(raw)
	assertPlanValidationError(t, err, "epic.title", "must be non-empty")
}

func TestParseGeneratedIntakePlanParsesValidJSON(t *testing.T) {
	t.Parallel()

	raw := `{
  "epic": {
    "title": "Build intake command",
    "description": "Turn ideas into epics and tasks",
    "priority": "high"
  },
  "tasks": [
    {
      "title": "Create parser",
      "description": "Parse generated JSON into intakePlan",
      "acceptance_criteria": ["Parser accepts schema-valid JSON"],
      "local_dependency_refs": ["task-2"]
    },
    {
      "title": "Create prompt template",
      "description": "Add a reusable prompt template",
      "acceptance_criteria": ["Template includes constraints"]
    }
  ]
}`

	plan, err := parseGeneratedIntakePlan(raw)
	if err != nil {
		t.Fatalf("parseGeneratedIntakePlan(valid) unexpected error: %v", err)
	}
	if plan.Epic.Title != "Build intake command" {
		t.Fatalf("epic title = %q, want %q", plan.Epic.Title, "Build intake command")
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if got := plan.Tasks[0].LocalDependencyRefs; len(got) != 1 || got[0] != "task-2" {
		t.Fatalf("task[0].local_dependency_refs = %#v, want [\"task-2\"]", got)
	}
}

func TestParseGeneratedIntakePlanRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	raw := `{"epic":{"title":"Build intake","description":"Desc","priority":"high"},"tasks":[}`
	_, err := parseGeneratedIntakePlan(raw)
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected wrapped json.SyntaxError, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "parse generated intake plan") {
		t.Fatalf("error text %q does not include parse context", err.Error())
	}
}

func TestBuildIntakePlanPromptRendersIdeaAndConstraints(t *testing.T) {
	t.Parallel()

	constraints := []string{
		"Prefer high-level tasks when requirements are uncertain.",
		"Include at least one concrete verification method per task.",
	}
	prompt, err := buildIntakePlanPrompt("Add yoke intake command for idea-to-epic generation", constraints)
	if err != nil {
		t.Fatalf("buildIntakePlanPrompt unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "Add yoke intake command for idea-to-epic generation") {
		t.Fatalf("rendered prompt missing idea text: %s", prompt)
	}
	for _, constraint := range constraints {
		if !strings.Contains(prompt, constraint) {
			t.Fatalf("rendered prompt missing constraint %q", constraint)
		}
	}
	if !strings.Contains(prompt, "Split oversized work into smaller tasks instead of targeting fixed task-count bounds.") {
		t.Fatalf("rendered prompt missing split-oversized-work guidance: %s", prompt)
	}
	if strings.Contains(prompt, "{{IDEA_TEXT}}") || strings.Contains(prompt, "{{GENERATION_CONSTRAINTS}}") {
		t.Fatalf("rendered prompt still includes template placeholders: %s", prompt)
	}
}

func TestGenerateIntakePlanRendersPromptAndParsesOutput(t *testing.T) {
	t.Parallel()

	idea := "Build intake command"
	constraints := []string{"Prefer incremental decomposition."}

	plan, err := generateIntakePlan(idea, constraints, func(prompt string) (string, error) {
		if !strings.Contains(prompt, idea) {
			t.Fatalf("generator prompt missing idea text: %s", prompt)
		}
		if !strings.Contains(prompt, constraints[0]) {
			t.Fatalf("generator prompt missing custom constraint: %s", prompt)
		}
		if !strings.Contains(prompt, splitOversizedWorkConstraint) {
			t.Fatalf("generator prompt missing default split-work guidance: %s", prompt)
		}

		return `{
  "epic": {
    "title": "Build intake command",
    "description": "Turn idea text into a structured plan",
    "priority": "high"
  },
  "tasks": [
    {
      "title": "Create prompt layer",
      "description": "Render prompt with constraints",
      "acceptance_criteria": ["Prompt includes idea and constraints"]
    }
  ]
}`, nil
	})
	if err != nil {
		t.Fatalf("generateIntakePlan unexpected error: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].Title != "Create prompt layer" {
		t.Fatalf("unexpected generated plan tasks: %#v", plan.Tasks)
	}
}

func TestValidateIntakePlanForApplyUsesSharedValidation(t *testing.T) {
	t.Parallel()

	err := validateIntakePlanForApply(intakePlan{
		Epic: validEpic(),
		Tasks: []intakePlanTask{
			{
				Title:              "Task title",
				Description:        "Task description",
				AcceptanceCriteria: []string{},
			},
		},
	})
	assertPlanValidationError(t, err, "tasks[0].acceptance_criteria", "must contain at least 1 item")
}

func assertPlanValidationError(t *testing.T, err error, wantPath, wantReason string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected validation error for %s", wantPath)
	}

	var validationErr *intakePlanValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected intakePlanValidationError, got %T (%v)", err, err)
	}
	if validationErr.Path != wantPath {
		t.Fatalf("validation error path = %q, want %q", validationErr.Path, wantPath)
	}
	if validationErr.Reason != wantReason {
		t.Fatalf("validation error reason = %q, want %q", validationErr.Reason, wantReason)
	}
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("error text %q does not include path %q", err.Error(), wantPath)
	}
	if !strings.Contains(err.Error(), wantReason) {
		t.Fatalf("error text %q does not include reason %q", err.Error(), wantReason)
	}
}

func validEpic() intakePlanEpic {
	return intakePlanEpic{
		Title:       "Epic title",
		Description: "Epic description",
		Priority:    "high",
	}
}

func validTask() intakePlanTask {
	return intakePlanTask{
		Title:              "Task title",
		Description:        "Task description",
		AcceptanceCriteria: []string{"One criterion"},
	}
}
