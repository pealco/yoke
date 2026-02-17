package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplyIntakePlanCreatesEpicTasksAndDependenciesInOrder(t *testing.T) {
	t.Parallel()

	plan := intakePlan{
		Epic: intakePlanEpic{
			Title:       "Epic title",
			Description: "Epic description",
			Priority:    "2",
		},
		Tasks: []intakePlanTask{
			{
				Ref:                "task-a",
				Title:              "Task A",
				Description:        "Task A description",
				AcceptanceCriteria: []string{"Task A criterion"},
			},
			{
				Ref:                 "task-b",
				Title:               "Task B",
				Description:         "Task B description",
				AcceptanceCriteria:  []string{"Task B criterion"},
				LocalDependencyRefs: []string{"task-a"},
			},
		},
	}

	var (
		recordedCalls [][]string
		createIndex   int
		createOutputs = []string{
			`{"id":"bd-epic-1"}`,
			`{"id":"bd-task-1"}`,
			`{"id":"bd-task-2"}`,
		}
	)

	runner := func(args ...string) (string, error) {
		recordedCalls = append(recordedCalls, append([]string(nil), args...))
		if len(args) == 0 {
			return "", errors.New("missing command")
		}
		switch args[0] {
		case "create":
			if createIndex >= len(createOutputs) {
				return "", errors.New("unexpected create call")
			}
			output := createOutputs[createIndex]
			createIndex++
			return output, nil
		case "dep":
			return "", nil
		default:
			return "", errors.New("unexpected command")
		}
	}

	result, err := applyIntakePlanWithRunner(plan, runner)
	if err != nil {
		t.Fatalf("applyIntakePlanWithRunner unexpected error: %v", err)
	}

	if result.EpicID != "bd-epic-1" {
		t.Fatalf("EpicID = %q, want bd-epic-1", result.EpicID)
	}
	if !reflect.DeepEqual(result.TaskIDs, []string{"bd-task-1", "bd-task-2"}) {
		t.Fatalf("TaskIDs = %#v, want [bd-task-1 bd-task-2]", result.TaskIDs)
	}

	expectedCalls := [][]string{
		{
			"create", "--type", "epic",
			"--title", "Epic title",
			"--description", "Epic description",
			"--priority", "2",
			"--json",
		},
		{
			"create", "--type", "task",
			"--title", "Task A",
			"--description", "Task A description",
			"--priority", "2",
			"--parent", "bd-epic-1",
			"--acceptance", "Task A criterion",
			"--json",
		},
		{
			"create", "--type", "task",
			"--title", "Task B",
			"--description", "Task B description",
			"--priority", "2",
			"--parent", "bd-epic-1",
			"--acceptance", "Task B criterion",
			"--json",
		},
		{"dep", "add", "bd-task-2", "bd-task-1"},
	}

	if !reflect.DeepEqual(recordedCalls, expectedCalls) {
		t.Fatalf("recorded calls = %#v, want %#v", recordedCalls, expectedCalls)
	}

	gotSummary := formatIntakeApplySummary(result)
	wantSummary := "Created epic: bd-epic-1\nCreated child tasks:\n1. bd-task-1\n2. bd-task-2"
	if gotSummary != wantSummary {
		t.Fatalf("summary = %q, want %q", gotSummary, wantSummary)
	}
}

func TestApplyIntakePlanDependencyValidationFailureSkipsDependencyWrites(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		plan          intakePlan
		errorContains string
	}{
		{
			name: "unknown dependency ref",
			plan: intakePlan{
				Epic: validEpic(),
				Tasks: []intakePlanTask{
					{
						Ref:                "task-a",
						Title:              "Task A",
						Description:        "Task A description",
						AcceptanceCriteria: []string{"Task A criterion"},
					},
					{
						Ref:                 "task-b",
						Title:               "Task B",
						Description:         "Task B description",
						AcceptanceCriteria:  []string{"Task B criterion"},
						LocalDependencyRefs: []string{"task-x"},
					},
				},
			},
			errorContains: "unknown local dependency ref",
		},
		{
			name: "duplicate dependency pair",
			plan: intakePlan{
				Epic: validEpic(),
				Tasks: []intakePlanTask{
					{
						Ref:                "task-a",
						Title:              "Task A",
						Description:        "Task A description",
						AcceptanceCriteria: []string{"Task A criterion"},
					},
					{
						Ref:                 "task-b",
						Title:               "Task B",
						Description:         "Task B description",
						AcceptanceCriteria:  []string{"Task B criterion"},
						LocalDependencyRefs: []string{"task-a", "task-a"},
					},
				},
			},
			errorContains: "duplicate dependency relation",
		},
		{
			name: "cycle",
			plan: intakePlan{
				Epic: validEpic(),
				Tasks: []intakePlanTask{
					{
						Ref:                 "task-a",
						Title:               "Task A",
						Description:         "Task A description",
						AcceptanceCriteria:  []string{"Task A criterion"},
						LocalDependencyRefs: []string{"task-b"},
					},
					{
						Ref:                 "task-b",
						Title:               "Task B",
						Description:         "Task B description",
						AcceptanceCriteria:  []string{"Task B criterion"},
						LocalDependencyRefs: []string{"task-a"},
					},
				},
			},
			errorContains: "cycle detected",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dependencyWrites := 0
			runner := func(args ...string) (string, error) {
				if len(args) >= 2 && args[0] == "dep" && args[1] == "add" {
					dependencyWrites++
				}
				return `{"id":"unused"}`, nil
			}

			_, err := applyIntakePlanWithRunner(tc.plan, runner)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.errorContains)
			}
			if !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.errorContains)
			}
			if dependencyWrites != 0 {
				t.Fatalf("dependencyWrites = %d, want 0", dependencyWrites)
			}
		})
	}
}
