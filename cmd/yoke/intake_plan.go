package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type intakePlan struct {
	Epic  intakePlanEpic   `json:"epic"`
	Tasks []intakePlanTask `json:"tasks"`
}

type intakePlanEpic struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

type intakePlanTask struct {
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	AcceptanceCriteria  []string `json:"acceptance_criteria"`
	LocalDependencyRefs []string `json:"local_dependency_refs,omitempty"`
}

type intakePlanValidationError struct {
	Path   string
	Reason string
}

func (e *intakePlanValidationError) Error() string {
	return fmt.Sprintf("intake plan validation failed at %s: %s", e.Path, e.Reason)
}

func parseGeneratedIntakePlan(raw string) (intakePlan, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()

	var plan intakePlan
	if err := decoder.Decode(&plan); err != nil {
		return intakePlan{}, fmt.Errorf("parse generated intake plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return intakePlan{}, fmt.Errorf("parse generated intake plan: unexpected trailing JSON")
	}
	if err := validateIntakePlan(plan); err != nil {
		return intakePlan{}, err
	}
	return plan, nil
}

func validateIntakePlanForApply(plan intakePlan) error {
	if err := validateIntakePlan(plan); err != nil {
		return fmt.Errorf("invalid intake plan for apply: %w", err)
	}
	return nil
}

func validateIntakePlan(plan intakePlan) error {
	if err := requireNonEmptyString(plan.Epic.Title, "epic.title"); err != nil {
		return err
	}
	if err := requireNonEmptyString(plan.Epic.Description, "epic.description"); err != nil {
		return err
	}
	if err := requireNonEmptyString(plan.Epic.Priority, "epic.priority"); err != nil {
		return err
	}
	if plan.Tasks == nil {
		return newIntakePlanValidationError("tasks", "is required")
	}
	// Intentionally only enforce a non-empty task-list requirement.
	if len(plan.Tasks) < 1 {
		return newIntakePlanValidationError("tasks", "must contain at least 1 task")
	}

	for i, task := range plan.Tasks {
		taskPath := fmt.Sprintf("tasks[%d]", i)
		if err := requireNonEmptyString(task.Title, taskPath+".title"); err != nil {
			return err
		}
		if err := requireNonEmptyString(task.Description, taskPath+".description"); err != nil {
			return err
		}
		if task.AcceptanceCriteria == nil {
			return newIntakePlanValidationError(taskPath+".acceptance_criteria", "is required")
		}
		if len(task.AcceptanceCriteria) == 0 {
			return newIntakePlanValidationError(taskPath+".acceptance_criteria", "must contain at least 1 item")
		}
		for j, criterion := range task.AcceptanceCriteria {
			if strings.TrimSpace(criterion) == "" {
				return newIntakePlanValidationError(
					fmt.Sprintf("%s.acceptance_criteria[%d]", taskPath, j),
					"must be non-empty",
				)
			}
		}
		for j, depRef := range task.LocalDependencyRefs {
			if strings.TrimSpace(depRef) == "" {
				return newIntakePlanValidationError(
					fmt.Sprintf("%s.local_dependency_refs[%d]", taskPath, j),
					"must be non-empty",
				)
			}
		}
	}

	return nil
}

func requireNonEmptyString(value, path string) error {
	if strings.TrimSpace(value) == "" {
		return newIntakePlanValidationError(path, "must be non-empty")
	}
	return nil
}

func newIntakePlanValidationError(path, reason string) error {
	return &intakePlanValidationError{
		Path:   path,
		Reason: reason,
	}
}
