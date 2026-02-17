package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type intakeApplyResult struct {
	EpicID  string
	TaskIDs []string
}

type intakeDependencyEdge struct {
	blockedRef string
	blockerRef string
}

type intakeBDRunner func(args ...string) (string, error)

func applyIntakePlan(plan intakePlan) (intakeApplyResult, error) {
	return applyIntakePlanWithRunner(plan, runIntakeBDCommand)
}

func applyIntakePlanWithRunner(plan intakePlan, run intakeBDRunner) (intakeApplyResult, error) {
	if run == nil {
		return intakeApplyResult{}, errors.New("nil bd runner")
	}

	if err := validateIntakePlanForApply(plan); err != nil {
		return intakeApplyResult{}, err
	}

	dependencyEdges, err := validateAndCollectDependencyEdges(plan)
	if err != nil {
		return intakeApplyResult{}, err
	}

	epicID, err := createBDIssue(
		run,
		"epic",
		plan.Epic.Title,
		plan.Epic.Description,
		plan.Epic.Priority,
		"",
		nil,
	)
	if err != nil {
		return intakeApplyResult{}, err
	}

	result := intakeApplyResult{
		EpicID:  epicID,
		TaskIDs: make([]string, 0, len(plan.Tasks)),
	}
	createdTaskIDsByRef := make(map[string]string, len(plan.Tasks))

	for i, task := range plan.Tasks {
		taskID, createErr := createBDIssue(
			run,
			"task",
			task.Title,
			task.Description,
			plan.Epic.Priority,
			epicID,
			task.AcceptanceCriteria,
		)
		if createErr != nil {
			return intakeApplyResult{}, fmt.Errorf("create task at tasks[%d]: %w", i, createErr)
		}
		result.TaskIDs = append(result.TaskIDs, taskID)
		createdTaskIDsByRef[strings.TrimSpace(task.Ref)] = taskID
	}

	for _, edge := range dependencyEdges {
		blockedID := createdTaskIDsByRef[edge.blockedRef]
		blockerID := createdTaskIDsByRef[edge.blockerRef]
		if _, depErr := run("dep", "add", blockedID, blockerID); depErr != nil {
			return intakeApplyResult{}, fmt.Errorf(
				"create dependency %s depends on %s: %w",
				edge.blockedRef,
				edge.blockerRef,
				depErr,
			)
		}
	}

	return result, nil
}

func formatIntakeApplySummary(result intakeApplyResult) string {
	var builder strings.Builder
	builder.WriteString("Created epic: ")
	builder.WriteString(result.EpicID)
	builder.WriteString("\nCreated child tasks:")
	for i, taskID := range result.TaskIDs {
		builder.WriteString(fmt.Sprintf("\n%d. %s", i+1, taskID))
	}
	return builder.String()
}

func validateAndCollectDependencyEdges(plan intakePlan) ([]intakeDependencyEdge, error) {
	knownTaskRefs := make(map[string]struct{}, len(plan.Tasks))
	for _, task := range plan.Tasks {
		knownTaskRefs[strings.TrimSpace(task.Ref)] = struct{}{}
	}

	edges := make([]intakeDependencyEdge, 0)
	seenPairs := make(map[string]struct{})
	for i, task := range plan.Tasks {
		blockedRef := strings.TrimSpace(task.Ref)
		for j, blockerRefRaw := range task.LocalDependencyRefs {
			blockerRef := strings.TrimSpace(blockerRefRaw)

			if _, exists := knownTaskRefs[blockerRef]; !exists {
				return nil, fmt.Errorf(
					"unknown local dependency ref %q at tasks[%d].local_dependency_refs[%d]",
					blockerRef,
					i,
					j,
				)
			}

			pairKey := blockedRef + "\x00" + blockerRef
			if _, exists := seenPairs[pairKey]; exists {
				return nil, fmt.Errorf("duplicate dependency relation %q depends on %q", blockedRef, blockerRef)
			}
			seenPairs[pairKey] = struct{}{}

			edges = append(edges, intakeDependencyEdge{
				blockedRef: blockedRef,
				blockerRef: blockerRef,
			})
		}
	}

	if err := detectDependencyCycle(plan, edges); err != nil {
		return nil, err
	}

	return edges, nil
}

func detectDependencyCycle(plan intakePlan, edges []intakeDependencyEdge) error {
	graph := make(map[string][]string, len(plan.Tasks))
	for _, task := range plan.Tasks {
		ref := strings.TrimSpace(task.Ref)
		graph[ref] = graph[ref]
	}
	for _, edge := range edges {
		graph[edge.blockedRef] = append(graph[edge.blockedRef], edge.blockerRef)
	}

	visitState := make(map[string]int, len(graph))
	var visit func(string) error
	visit = func(ref string) error {
		visitState[ref] = 1
		for _, dependencyRef := range graph[ref] {
			switch visitState[dependencyRef] {
			case 1:
				return fmt.Errorf("cycle detected involving local task ref %q", dependencyRef)
			case 0:
				if err := visit(dependencyRef); err != nil {
					return err
				}
			}
		}
		visitState[ref] = 2
		return nil
	}

	for _, task := range plan.Tasks {
		ref := strings.TrimSpace(task.Ref)
		if visitState[ref] != 0 {
			continue
		}
		if err := visit(ref); err != nil {
			return err
		}
	}

	return nil
}

func createBDIssue(
	run intakeBDRunner,
	issueType, title, description, priority, parent string,
	acceptanceCriteria []string,
) (string, error) {
	args := []string{
		"create",
		"--type", issueType,
		"--title", title,
		"--description", description,
		"--priority", priority,
	}
	if parent != "" {
		args = append(args, "--parent", parent)
	}
	if len(acceptanceCriteria) > 0 {
		args = append(args, "--acceptance", strings.Join(acceptanceCriteria, "\n"))
	}
	args = append(args, "--json")

	output, err := run(args...)
	if err != nil {
		return "", fmt.Errorf("bd create (%s %q): %w", issueType, title, err)
	}

	createdID, parseErr := parseCreatedIssueID(output)
	if parseErr != nil {
		return "", fmt.Errorf("parse created issue id: %w", parseErr)
	}
	return createdID, nil
}

func parseCreatedIssueID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty output")
	}

	var issue struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &issue); err == nil {
		if strings.TrimSpace(issue.ID) != "" {
			return issue.ID, nil
		}
	}

	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &issues); err == nil {
		if len(issues) > 0 && strings.TrimSpace(issues[0].ID) != "" {
			return issues[0].ID, nil
		}
	}

	var issueID string
	if err := json.Unmarshal([]byte(trimmed), &issueID); err == nil {
		if strings.TrimSpace(issueID) != "" {
			return issueID, nil
		}
	}

	if fields := strings.Fields(trimmed); len(fields) == 1 {
		return fields[0], nil
	}

	return "", fmt.Errorf("unsupported output format: %q", trimmed)
}

func runIntakeBDCommand(args ...string) (string, error) {
	return commandOutput("bd", args...)
}
