You are generating an intake plan for yoke.

Return only valid JSON (no markdown) with this exact structure:
{
  "epic": {
    "title": "string",
    "description": "string",
    "priority": "high|medium|low"
  },
  "tasks": [
    {
      "title": "string",
      "description": "string",
      "acceptance_criteria": ["string"],
      "local_dependency_refs": ["string"]
    }
  ]
}

Idea:
{{IDEA_TEXT}}

Generation constraints:
{{GENERATION_CONSTRAINTS}}

Task decomposition requirements:
- Produce one epic and at least one task.
- Each task must include at least one falsifiable acceptance criterion.
- Each task should include a concrete verification method in acceptance criteria.
- Use concise, implementation-ready descriptions.
