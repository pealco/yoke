package main

import (
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

func TestValidateIntakePlanForApplyRequiresTaskRefs(t *testing.T) {
	t.Parallel()

	err := validateIntakePlanForApply(intakePlan{
		Epic: validEpic(),
		Tasks: []intakePlanTask{
			{
				Title:              "Task title",
				Description:        "Task description",
				AcceptanceCriteria: []string{"One criterion"},
			},
		},
	})
	assertPlanValidationError(t, err, "tasks[0].ref", "must be non-empty")
}

func TestValidateIntakePlanForApplyRejectsDuplicateTaskRefs(t *testing.T) {
	t.Parallel()

	err := validateIntakePlanForApply(intakePlan{
		Epic: validEpic(),
		Tasks: []intakePlanTask{
			{
				Ref:                "task-a",
				Title:              "Task A",
				Description:        "Task A description",
				AcceptanceCriteria: []string{"A criterion"},
			},
			{
				Ref:                "task-a",
				Title:              "Task B",
				Description:        "Task B description",
				AcceptanceCriteria: []string{"B criterion"},
			},
		},
	})
	assertPlanValidationError(t, err, "tasks[1].ref", "must be unique (duplicates tasks[0].ref)")
}

func TestValidateIntakePlanForApplyRejectsEmptyLocalDependencyRef(t *testing.T) {
	t.Parallel()

	err := validateIntakePlanForApply(intakePlan{
		Epic: validEpic(),
		Tasks: []intakePlanTask{
			{
				Ref:                "task-a",
				Title:              "Task A",
				Description:        "Task A description",
				AcceptanceCriteria: []string{"A criterion"},
			},
			{
				Ref:                 "task-b",
				Title:               "Task B",
				Description:         "Task B description",
				AcceptanceCriteria:  []string{"B criterion"},
				LocalDependencyRefs: []string{" "},
			},
		},
	})
	assertPlanValidationError(t, err, "tasks[1].local_dependency_refs[0]", "must be non-empty")
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
