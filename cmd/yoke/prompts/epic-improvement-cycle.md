## Overview

Create an agent team to thoroughly review, proofread, refine, and polish a beads epic so implementation is smooth, intent is preserved, and quality/operability gates run during and after implementation.

Core goals:
- Make every task unambiguous, falsifiable, and verifiable.
- Make dependencies correct, minimal, and non-circular.
- Right-size scope to coherent, single-session work units.
- Inject idempotent quality gates (no duplicate blocks on re-run).
- Add final epic gates for code review and operational readiness.

## Non-negotiable rules

1) **Do not invent requirements.** If intent is unclear, create a clarification task and block implementation on it.
2) **Be idempotent.** If a gate block already exists (markers), update in-place; do not add duplicates.
3) **Preserve intent.** Only rewrite text to increase clarity/precision unless the epic contradicts itself or is factually stale. When you must change meaning, surface it explicitly in the report.
4) **Acceptance criteria must be falsifiable.** If a criterion cannot be proven true/false, rewrite it (or split the task).
5) **Avoid scope creep while enforcing quality.** Only require tests/verification that cover behavioral changes introduced by the task (or gaps exposed by those changes).

## Process

1. **Load the epic**
   - Run: `bd show $EPIC_ID`
   - Review the epic description and all linked tasks/subtasks.

2. **Classify each task**
   For each task, assign one of:
   - **CODE/BEHAVIORAL:** Changes production behavior, logic, APIs, error handling, performance characteristics, security behavior, or other user-visible/system-visible semantics.
   - **NON-CODE/OPS/DOC/DATA:** Documentation, configs with no behavioral code paths, dashboards/alerts, vendor coordination, one-off data operations, or purely mechanical changes.

   If classification is ambiguous, treat as CODE/BEHAVIORAL by default unless the task explicitly states it is non-code.

3. **Review each task for clarity and correctness**
   For every task, ensure:
   - **Clear acceptance criteria** (falsifiable and verifiable).
   - **No contradictions** within the task or against the epic.
   - **No stale references** (renamed modules, removed endpoints, outdated paths). If unsure, create a clarification task rather than guessing.
   - **Consistent terminology** across the epic (names of modules, concepts, error types).
   - **Dependencies are correct** (nothing missing, nothing circular; see step 6).
   - **Scope is right-sized** (see step 5).

4. **Upgrade acceptance criteria to be verifiable**
   Rewrite acceptance criteria so each item is checkable by at least one of:
   - A test (unit/integration/e2e/contract).
   - A concrete command with expected output (including exit code, logs, or diff).
   - Clear UI steps and expected observable result.
   - Concrete input/output examples (including error cases).

   If a criterion is vague (“robust,” “handle edge cases,” “improve performance”), replace with:
   - Explicit cases + expected outcomes (including error paths).
   - Boundaries (limits, timeouts, concurrency behavior where relevant).
   - If performance-related, define measurement method and threshold.

5. **Break up complex tasks (mechanical sizing rubric)**
   Split a task if ANY of the following are true:
   - It spans multiple unrelated subsystems (e.g., DB + UI + auth) without a single coherent objective.
   - It would reasonably require more than one PR to keep reviewable.
   - It includes both “decide/align” and “implement” in one unit (split decision/contract from implementation).
   - Acceptance criteria include multiple independent deliverables that can be completed and reviewed separately.

   When splitting:
   - Create new tasks with `bd create`.
   - Set dependencies with `bd dep add`.
   - Update the original task to either become a smaller, coherent task or explicitly become an umbrella task that depends on the new tasks.

6. **Dependency sanity pass (graph correctness)**
   Ensure:
   - No circular dependencies.
   - Each task has prerequisites it truly needs (avoid over-blocking).
   - If a task depends on an implicit prerequisite that does not exist, create a task for it (or a clarification task) and add the dependency.
   - Where many tasks converge, consider an explicit late-stage integration task if appropriate (e.g., “Integrate changes and run full suite”), but keep it minimal.

   In the report, list the critical dependency chains that determine the epic’s longest path.

7. **Create “Clarification needed” tasks when required**
   If any task requires product/behavior intent that is not specified, or there’s a factual uncertainty you cannot resolve from the epic text:
   - Create a new task:
     - **Title:** `Clarification needed: <short question>`
     - **Type:** `task`
     - **Description:** State the question(s), why they matter, and what tasks are blocked until answered.
   - Add dependencies so ambiguous implementation tasks are blocked by the clarification task.

8. **Inject per-task quality gates (idempotent upsert)**
   For each task:
   - If it is **CODE/BEHAVIORAL**, inject the CODE gate block.
   - If it is **NON-CODE/OPS/DOC/DATA**, inject the NON-CODE gate block.
   - If the task already contains markers for the relevant gate, replace the content between markers with the latest template.
   - Ensure the gate appears after acceptance criteria.

   IMPORTANT: Do not inject both gate types into the same task. Choose the one that matches classification.

9. **Inject post-epic gates (as final tasks)**
   Add two final tasks:
   A) **Post-epic code review** (blocked by all other tasks)
   B) **Post-epic operational readiness review** (blocked by all other tasks AND blocked by the post-epic code review)

   Use `bd create` and `bd dep add`. Priority should match the epic.

10. **Add/upgrade epic-level Definition of Done (DoD)**
   Add an epic-level DoD block in the epic description (idempotent upsert using markers) that states what “epic done” means beyond “tasks closed.”

11. **Proofread**
   Fix typos, ambiguous phrasing, and unclear titles. Normalize terminology.

12. **Report**
   Summarize changes with traceability (task IDs/titles). Include before/after rubric scores with justification.

---

## Templates (copy exactly; idempotent markers required)

### A) CODE/BEHAVIORAL per-task gate (inject after acceptance criteria)

<!-- IMPROVE_EPIC_TASK_GATE:CODE:v1 -->
**REQUIRED:** Use superpowers:test-driven-development when implementing this task.

**POST-TASK: Test coverage review**
After all acceptance criteria are met and tests pass, systematically review whether the test suite covers the behavioral changes introduced by this task. This is part of completing the task.

Specifically:
- Read each test file that was modified or should cover the changed code.
- For each behavioral difference introduced (new error types, new semantics, changed control flow), verify a test exercises that path.
- If a gap exists and it was introduced or exposed by your changes, write the test. Do not punt to a follow-up task.
- If a gap is pre-existing and unrelated to your changes, note it when closing the task but do not fix it.
- Test edge cases: invalid inputs, error paths, boundary conditions, concurrency (if applicable).

**POST-TASK: Rollout/rollback note (when applicable)**
If this task changes runtime behavior behind a flag, config, schema, API contract, or deployment workflow, include:
- Rollout steps (flags/config/migrations).
- Rollback steps (how to revert safely).
<!-- /IMPROVE_EPIC_TASK_GATE:CODE:v1 -->

### B) NON-CODE/OPS/DOC/DATA per-task gate (inject after acceptance criteria)

<!-- IMPROVE_EPIC_TASK_GATE:NONCODE:v1 -->
**POST-TASK: Verification plan**
Before closing:
- Provide a concrete verification method (command + expected output, checklist of observable UI results, or a data validation query + expected results).
- Record what evidence confirms success (logs/screenshots/output snippets as appropriate).

**POST-TASK: Rollback plan**
If this task affects production operations (configs, flags, dashboards/alerts, data operations):
- State how to revert safely.
- State any risks or irreversible steps.
<!-- /IMPROVE_EPIC_TASK_GATE:NONCODE:v1 -->

### C) Epic Definition of Done (insert into epic description)

<!-- IMPROVE_EPIC_DOD:v1 -->
## Definition of Done
The epic is complete only when:
- All tasks are closed with evidence of completion (tests/commands/results or verification checklist).
- All CODE/BEHAVIORAL tasks include appropriate tests or an explicit, justified alternative verification plan.
- No open “Clarification needed” tasks remain unless explicitly accepted as out of scope for this epic.
- Post-epic code review task is closed (with any must-fix issues resolved).
- Post-epic operational readiness review task is closed (deploy/rollback/monitoring/runbooks addressed as needed).
<!-- /IMPROVE_EPIC_DOD:v1 -->

---

## Post-epic tasks (create these as new tasks)

### 1) Post-epic code review: <epic title>
- **Type:** task
- **Priority:** same as the epic
- **Dependencies:** blocked by all other tasks in the epic

**Description (copy exactly):**
All implementation tasks in this epic are complete. Before declaring the epic done, perform a thorough code review of every file changed across the entire epic.

**Mindset:** Read the code like a skeptical, senior engineer. Assume nothing works until you’ve read it. Look for things that are wrong, not things that are right.

**Review checklist:**
- Inconsistent patterns across tasks (style, error handling, duplicated logic).
- Integration bugs at task boundaries (shared modules, common utilities, configuration).
- Unnecessary complexity or over-engineering.
- Dead code and leftover artifacts (unused imports, commented code, orphan helpers, stale dependencies).
- Error handling gaps where modules touch (changed error types/assumptions).
- Test coverage at integration seams (where output from one changed module feeds into another).

**Issue triage (avoid scope explosion):**
- Must-fix before close: correctness, security, data loss/corruption, crashes, broken tests, deploy risk, silent failure modes, broken observability.
- Can defer (create issue, don’t block close): refactor-only improvements, minor cleanup, purely stylistic nits.

**Output:**
- For each must-fix issue, create a beads issue (`bd create --type=bug`) and ensure it is resolved before closing this task.
- Minor nits can be fixed inline without separate issues.
- Close this task only after all must-fix issues are resolved.

### 2) Post-epic operational readiness review: <epic title>
- **Type:** task
- **Priority:** same as the epic
- **Dependencies:** blocked by all other tasks AND blocked by “Post-epic code review: <epic title>”

**Description (copy exactly):**
All implementation tasks and the post-epic code review are complete. Before declaring the epic done, confirm the change is operable in production.

**Operational readiness checklist:**
- Rollout plan exists and is correct (flags/config/migrations/deploy steps as applicable).
- Rollback plan exists and is safe (revert path, migration rollback strategy if needed).
- Observability is adequate for new/changed behavior:
  - Logs for new error paths or key state transitions.
  - Metrics/traces where appropriate (especially for failure modes).
  - Alerts/dashboards updated if the failure surface changed.
- Runbooks/docs updated if oncall or operators will need new procedures.
- Backward compatibility validated (API/schema/contracts) or explicitly documented if not supported.

**Output:**
- If any must-fix gap exists, create a beads bug (`bd create --type=bug`) and ensure it is resolved before closing this task.
- Close this task only when the checklist is satisfied or explicitly documented as not applicable (with justification).

---

## Report format (must follow)

1) **What changed (traceable)**
- Tasks created (ID, title, why).
- Tasks split/rewritten (ID, title, what changed, why).
- Dependencies added/removed (A -> B, why).
- Clarification tasks created (ID, question, what is blocked).

2) **Open questions / remaining risks**
- Explicit list, with task IDs that depend on answers.

3) **Dependency summary**
- List the critical dependency chains (longest paths) and any integration/seam hotspots.

4) **Before/after quality score (rubric)**
Score 0–10 using five dimensions (0–2 each). Provide before and after with 1–2 sentences justification per dimension.
- Acceptance criteria verifiable (0–2)
- Dependencies coherent/minimal (0–2)
- Scope right-sized/cohesive (0–2)
- Unknowns surfaced + blocked appropriately (0–2)
- Operability addressed (rollout/rollback/observability/docs) (0–2)
