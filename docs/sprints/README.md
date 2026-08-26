# Sprint Backlogs — index

Detailed, agent-ready task cards for each milestone in [`../ROADMAP.md`](../ROADMAP.md).

Every task follows the template:
- **Goal** — one paragraph
- **Files to touch** — specific paths
- **Approach** — step-by-step outline (agent-executable)
- **Acceptance criteria (DoD)** — measurable checkbox list
- **Effort** — S (< 1 day), M (1-2 days), L (2-5 days)
- **Dependencies** — task IDs this depends on

## Files

| File | Milestone | Sprints | Tasks |
|---|---|---:|---:|
| [`M0-fix-broken-strategies.md`](M0-fix-broken-strategies.md) | M0 — Fix broken strategies (prereq) | 1 | 7 |
| [`M1-correctness.md`](M1-correctness.md) | M1 — Correctness first | 3 | 8 |
| [`M2-richness.md`](M2-richness.md) | M2 — Rich alternatives | 3 | 6 |
| [`M3-enrichment.md`](M3-enrichment.md) | M3 — Full enrichment | 2 | 4 |
| [`M4-data-sources.md`](M4-data-sources.md) | M4 — Beyond TecDoc | 4 | 10 |
| [`M5-M6-intelligence-and-production.md`](M5-M6-intelligence-and-production.md) | M5 + M6 | 5 | 10 |
| [`M8-online-search-aggregation.md`](M8-online-search-aggregation.md) | M8 — Online-search aggregation (free/public sources for HK aftermarket) | 12 | 12 |

## Workflow

1. **Pick a task** — copy the task ID (e.g. `M1.S1.T1`) into a fresh agent chat.
2. **Read the task** — every task is self-contained; file paths + approach + DoD are enough context.
3. **Branch** — `git checkout main && git pull && git checkout -b {milestone}/{task-slug}` (per CLAUDE.md rebase rule).
4. **Execute** — write code + tests to satisfy every DoD checkbox.
5. **Verify** — `go build ./... && go vet ./... && go test ./...` all pass; audit re-run if the task claims a metric shift.
6. **Commit trailer** — include `Milestone: {M}.{S}` and `Task: {task-id}` so `git log --grep 'Milestone: M1'` scopes correctly.
7. **PR** — reference the task by ID in the PR title (e.g. `[M1.S1.T1] feat(ranker): strategyCategoryPenalty for cross-family mismatches`).

## Task-scoping rules

- Every task should be **~50-150 lines of production code + tests**.
- Every task should be **reviewable in ≤ 1 hour**.
- Every task must have a **measurable exit criterion** (metric shift, test passes, endpoint returns X).
- **Never bundle tasks across sprints in the same PR.** Cross-cutting infrastructure (new tables, migrations, brand-normalization) can span tasks within a sprint.

## Blocked tasks

If a task blocks on:
- **External partnership** (dealer catalog): mark `blocked-external` in the task title; skip and pick the next unblocked task.
- **A prior task** that hasn't shipped: the "Dependencies" line names it; pick a different task.
- **A spike / research** that returns "not feasible": document the finding in `docs/data-sources/` and mark the parent milestone with a `blocked-on-decision` note.
