# Engine health check — unified audit + diagnostic runbook

One command that runs the API quality audit AND surfaces the TecDoc MySQL
diagnostic together, then combines them into a single dated report at
`docs/reports/{date}-engine-health/summary.md`.

Ships with PR "combine audit + diagnostics" (this PR). Depends on the
scripts introduced in PR #22 (`tecdoc_health_report_min.sql` +
`sql/08_articlecriteria_criteria_value_hotfix.sql`).

---

## When to run this

| Trigger | Why |
|---|---|
| After deploying a new build to qa | Confirm no regression on any of the north-star metrics |
| After applying a DB migration | Verify the new index / column is used by the query planner |
| Weekly | Trend-track F1_correct, AvgAM_correct, guard leaks against the baseline |
| Before opening any PR that touches `internal/service/` | Get a fresh "before" number to cite in the PR body |
| When something in the audit dashboards looks off | Baseline + delta report to isolate what changed |

---

## What the script does

Three stages, one command:

```
┌────────────────────────────────────────────────────────────────────┐
│                                                                    │
│   ┌─────────────────┐    ┌──────────────────┐   ┌───────────────┐  │
│   │ audit-quality   │───▶│ analyze-quality  │──▶│  summary.md   │  │
│   │  200-canary     │    │  by-slice / cat  │   │  (auto)       │  │
│   └─────────────────┘    └──────────────────┘   └───────┬───────┘  │
│           API audit                                     │          │
│                                                         ▼          │
│   ┌─────────────────┐                             ┌──────────┐     │
│   │  sql/08 apply   │─┐                           │          │     │
│   └─────────────────┘ │                           │          │     │
│   ┌─────────────────┐ ├──▶ operator pastes ──────▶│  paste   │     │
│   │  tecdoc-min.sql │ │    output.txt into        │  slot    │     │
│   └─────────────────┘ │    summary.md             │          │     │
│           DB half     │                           │          │     │
│           (manual)    │                           └──────────┘     │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

- **Stage 1** — runs `scripts/audit/audit-quality.ps1` against the API
- **Stage 2** — runs `scripts/audit/analyze-quality.ps1` on the fresh raw CSV
- **Stage 3** — prints the `mysql` commands you need to run manually for the
  DB diagnostic (the script never handles DB creds). Also writes a
  ready-to-fill markdown template with a paste slot for the DB output

---

## Quick start (after PR #22 merges)

### Option A — the canonical qa post-deploy check

```pwsh
pwsh scripts/engine-health-check.ps1 `
  -ApiUrl https://qa.ifritah.com `
  -Corpus scripts/audit/corpus-200-canary.csv
```

Runtime ~3 min for the API audit, then paste the mysql output into
`docs/reports/{date}-engine-health/summary.md`.

### Option B — full 1490-corpus regression (before shipping to prod)

```pwsh
pwsh scripts/engine-health-check.ps1 `
  -ApiUrl https://qa.ifritah.com `
  -Corpus scripts/audit/corpus-1500-v2.csv `
  -BaselineCsv scripts/audit/qa-quality-by-slice-2026-08-25_1817.csv
```

Runtime ~25 min. Emits the same combined report with a delta section
pointing at the baseline.

### Option C — DB-only day (no API audit)

```pwsh
pwsh scripts/engine-health-check.ps1 -SkipAudit
```

Just prints the mysql commands and writes the template with the DB
paste slot.

### Option D — audit only, skip DB instructions

```pwsh
pwsh scripts/engine-health-check.ps1 -SkipDb
```

---

## Parameters

| Param | Default | Purpose |
|---|---|---|
| `-ApiUrl` | `https://qa.ifritah.com` | Target API |
| `-Corpus` | `scripts/audit/corpus-200-canary.csv` | Input OEM list |
| `-Modes` | `combined` | Strategy modes to test (comma-separated) |
| `-Enrichment` | `full` | Enrichment level (`full` / `basic` / `none`) |
| `-BaselineCsv` | (empty) | Path to prior `by-slice.csv` for delta comparison |
| `-SkipAudit` | off | Skip API audit stage (DB-only run) |
| `-SkipDb` | off | Skip DB instructions (API-only run) |
| `-ReportDir` | `docs\reports\{date}-engine-health` | Where to write `summary.md` |

---

## What the DB half does (manual)

You need direct access to the TecDoc MySQL — the script never handles creds.

### Step 1 — Apply the hotfix migration (idempotent)

```bash
mysql --host=<tecdoc-mysql-host> --user=<user> --password --database=<db> \
      < sql/08_articlecriteria_criteria_value_hotfix.sql
```

5-15 min DDL on 27M rows, online, no application downtime.

### Step 2 — Run the minimal diagnostic

```bash
mysql --host=<tecdoc-mysql-host> --user=<user> --password --database=<db> \
      < scripts/diagnostics/tecdoc_health_report_min.sql \
      > docs/reports/{date}-engine-health/tecdoc-min-{timestamp}.txt
```

Runtime ~30 seconds. Answers 7 questions:

| § | Question |
|---|---|
| A | Do the 19 real HK corpus OEMs resolve via `oem_number`? |
| B | What REAL aftermarket brands appear per corpus OEM? |
| C | Do the corpus articles have supersession chains? |
| D | Do they have specs? |
| E | HK vehicle catalog (Hyundai / Kia / Genesis linkage IDs) |
| F | Language distribution |
| G | EXPLAIN plans — G1 confirms `sql/08` index is used by the planner |

### Step 3 — Paste the output

The script pre-generates `docs/reports/{date}-engine-health/summary.md` with a
paste slot fenced by `<!-- BEGIN tecdoc-min output -->` and
`<!-- END tecdoc-min output -->`. Drop the `.txt` output between the fences.

---

## What the combined report contains

```
docs/reports/{date}-engine-health/
├── summary.md                 ← auto-generated, has paste slot for DB output
├── tecdoc-min-{ts}.txt        ← operator-created (mysql output)
├── qa-quality-raw-{ts}.csv    ← audit raw
├── qa-quality-summary-{ts}.md ← audit summary
├── qa-quality-by-slice-{ts}.csv
└── qa-quality-by-category-{ts}.csv
```

`summary.md` structure:

1. **Environment** — API URL, corpus, modes, timestamp
2. **Stage 1: API audit** — auto-populated from the audit summary
3. **Stage 2: TecDoc MySQL diagnostic** — paste slot for the DB output
4. **Stage 3: Delta vs baseline** — if `-BaselineCsv` provided
5. **Next actions** — checklist for follow-up work

---

## Why this exists

Before this script, the "engine health" workflow was three separate scripts run
in three separate windows, with results scattered across three places (raw CSV
in `scripts/audit/`, DB output in a Terminal buffer, delta comparison done
by eye). No single artifact could be pointed at when someone asked "how is
the engine doing right now?".

After this script, one command produces one artifact
(`docs/reports/{date}-engine-health/summary.md`) that answers the question in
one page.

## Relationship to PR #22

PR #22 shipped the DB-side diagnostic scripts and the sql/08 hotfix. This
runbook is the API-side and orchestration counterpart — you run PR #22's
SQL for the DB half and this script for the API half; the script pulls both
into one combined report.

## Related files

- `scripts/audit/audit-quality.ps1` — raw API audit (used internally by the runbook)
- `scripts/audit/analyze-quality.ps1` — analyzer (used internally by the runbook)
- `scripts/diagnostics/tecdoc_health_report_min.sql` — DB diagnostic (PR #22)
- `sql/08_articlecriteria_criteria_value_hotfix.sql` — DB migration (PR #22)
- `docs/reports/TEMPLATE-engine-health.md` — the report template the script generates
