package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const splitOversizedWorkConstraint = "Split oversized work into smaller tasks instead of targeting fixed task-count bounds."

//go:embed prompts/intake-plan.md
var intakePlanPromptTemplate string

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

func buildIntakePlanPrompt(idea string, constraints []string) (string, error) {
	if strings.TrimSpace(intakePlanPromptTemplate) == "" {
		return "", errors.New("intake plan prompt template is empty")
	}

	trimmedIdea := strings.TrimSpace(idea)
	if trimmedIdea == "" {
		return "", errors.New("idea text is required")
	}

	prompt := strings.ReplaceAll(intakePlanPromptTemplate, "{{IDEA_TEXT}}", trimmedIdea)
	prompt = strings.ReplaceAll(prompt, "{{GENERATION_CONSTRAINTS}}", formatIntakePlanConstraints(constraints))
	return prompt, nil
}

func formatIntakePlanConstraints(constraints []string) string {
	combined := make([]string, 0, len(constraints)+1)
	combined = append(combined, splitOversizedWorkConstraint)
	for _, constraint := range constraints {
		trimmed := strings.TrimSpace(constraint)
		if trimmed == "" {
			continue
		}
		combined = append(combined, trimmed)
	}

	lines := make([]string, 0, len(combined))
	for _, constraint := range combined {
		lines = append(lines, "- "+constraint)
	}
	return strings.Join(lines, "\n")
}

type intakePlanGenerator func(prompt string) (string, error)

func generateIntakePlan(idea string, constraints []string, generator intakePlanGenerator) (intakePlan, error) {
	if generator == nil {
		return intakePlan{}, errors.New("intake plan generator is required")
	}

	prompt, err := buildIntakePlanPrompt(idea, constraints)
	if err != nil {
		return intakePlan{}, err
	}

	raw, err := generator(prompt)
	if err != nil {
		return intakePlan{}, fmt.Errorf("generate intake plan: %w", err)
	}

	return parseGeneratedIntakePlan(raw)
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
