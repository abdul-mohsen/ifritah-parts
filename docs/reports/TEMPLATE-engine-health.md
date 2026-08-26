# Engine Health Report — {date}

_Template for `scripts/engine-health-check.ps1` output. The script fills this
in automatically; edit only if you want to tune the shape for a specific
audit run._

---

## Environment

| Setting | Value |
|---|---|
| API URL | `https://qa.ifritah.com` |
| Corpus | `scripts/audit/corpus-200-canary.csv` |
| Modes | `combined` |
| Enrichment | `full` |
| Timestamp | `{yyyy-MM-dd_HHmm}` |
| sql/08 applied? | (yes/no + when) |
| strategy_spec_match fix applied? | (yes/no + PR link) |

---

## Stage 1: API quality audit

Raw CSV: `qa-quality-raw-{ts}.csv`
Summary: `qa-quality-summary-{ts}.md`
Per-slice CSV: `qa-quality-by-slice-{ts}.csv`

### Headline numbers

| Metric | This run | Previous run | Baseline (2026-08-25) | Δ vs baseline |
|---|---:|---:|---:|---:|
| `F1_correct` overall | | | 0.25 | |
| `F1_correct` seeded slice | | | 0.71 | |
| `AvgAM_correct` on wear parts | | | 0.09 | |
| `AvgOEMxRef_correct` | | | ~0 | |
| `F1_rich5` on wear parts | | | ~0 | |
| Non-HK guard leaks | | | 10/100 | |
| Timeouts per 100 | | | 3 | |
| p95 latency (spec queries) | | | ~3 s | |

_(Copy the actual numbers out of the auto-generated summary section below.)_

### Auto-generated audit summary

<!-- BEGIN audit summary -->
_(script inlines qa-quality-summary-{ts}.md here)_
<!-- END audit summary -->

---

## Stage 2: TecDoc MySQL diagnostic

Applied migrations before this run:

- [ ] `sql/08_articlecriteria_criteria_value_hotfix.sql`
- [ ] (list any other DB-side changes)

<!-- BEGIN tecdoc-min output -->
```
(paste tecdoc-min-{ts}.txt here — full output of tecdoc_health_report_min.sql)
```
<!-- END tecdoc-min output -->

### Section-by-section verdict

| § | Question | Verdict |
|---|---|---|
| A | Corpus OEMs resolve in `oem_number`? | ✅ / ⚠️ / ❌ — X of 19 hit |
| B | Real aftermarket brands per corpus OEM? | ✅ / ⚠️ / ❌ — brands: (list) |
| C | Supersession chain coverage? | ✅ / ⚠️ / ❌ — X of Y articles |
| D | Spec coverage? | ✅ / ⚠️ / ❌ — X of Y have specs |
| E | HK vehicle catalog present? | ✅ / ⚠️ / ❌ — X linkage IDs |
| F | Language distribution matches app assumptions? | ✅ / ⚠️ / ❌ |
| G | sql/08 index used by planner? | ✅ / ⚠️ / ❌ — key= |

---

## Stage 3: Delta vs baseline

Baseline used: `qa-quality-by-slice-{yyyy-MM-dd_HHmm}.csv`

### What moved

- **Improved**: (list metrics that went up)
- **Regressed**: (list metrics that went down — MUST include a plan to fix)
- **No change**: (list metrics that didn't move — is that expected?)

### Root-cause hypotheses for anything that regressed

1. …
2. …

---

## Next actions

Based on this run, the follow-up tasks and owners:

| Priority | Task | Owner | Est |
|---|---|---|---|
| P0 | | | |
| P1 | | | |
| P2 | | | |

---

## Related PRs / commits

- Post-run commit: `{hash}` — this report + any code fixes
- Related PR: #{n} — {title}
- Baseline audit: `docs/reports/{prior-date}-engine-health/summary.md`

## Post-mortem (optional)

If this run surfaces a regression that required a rollback, document it here
so the next agent has the context:

- **What went wrong**:
- **Why it wasn't caught earlier**:
- **How we'll catch it next time** (test / CI gate / alert):
