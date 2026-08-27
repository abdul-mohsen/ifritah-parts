# Nightly search-quality audit reports

This folder holds the output of the automated **nightly search-quality
audit** — the guardrail against silent regression of the parts search
engine on qa.ifritah.com. One subfolder per UTC date, e.g.
`2026-08-28/`, containing the raw + rolled-up CSVs plus a plain-text
summary produced by `scripts/audit/analyze-quality.ps1`.

Workflow that publishes this: **`.github/workflows/nightly-audit.yml`**
(schedule: `0 3 * * *` UTC daily; also `workflow_dispatch` for manual
runs). Corpus: **`scripts/audit/corpus-canary-v1.csv`** — 200 OEMs
weighted toward wear parts + real seeded Hyundai/Kia ground truth, with
a handful of non-HK deny-list samples so the guard-leak metric stays
alive.

---

## What each subfolder contains

Each `docs/reports/nightly/{yyyy-MM-dd}/` folder is written by the
workflow after a successful qa run and holds the following files
(names normalised — the raw scripts emit timestamped filenames; the
workflow strips the timestamp for stable diff-ability):

| File                       | What                                                                                        |
|----------------------------|---------------------------------------------------------------------------------------------|
| `raw.csv`                  | One row per OEM: response fields + enrichment counts. Regenerated every run.                |
| `by-category.csv`          | Grouped by `ExpectedCategory`. All Tier-1/Tier-2/Tier-3 metrics per category.               |
| `by-system.csv`            | Grouped by `ExpectedSystem` (Brakes / Engine / HVAC / …). Higher-level view than category.  |
| `by-slice.csv`             | Grouped by corpus `Slice` (real_hk_seeded / real_hk_coarse / plausible_hk / non_hk / …).    |
| `by-strategy.csv`          | Grouped by the strategy that produced the top hit (`SourceStrategy`). Which one broke?      |
| `by-strategy-slice.csv`    | `SourceStrategy × Slice`. Cross-tab for "did strategy X regress on slice Y".                |
| `failures.csv`             | Every OEM that missed `F1_correct` OR was correct but had `replTotal < 3`. Debug-first.     |
| `summary.txt`              | Human-readable overview of the run: top failing categories, floor breaches, alert reasons.  |

Files are produced by `scripts/audit/analyze-quality.ps1`. See
[`scripts/audit/README.md`](../../../scripts/audit/README.md) for the
audit runner's parameter reference and the corpus-column contract.

---

## North-star metrics — what they mean

The analyzer emits three tiers of metrics. In priority order:

### Tier 1 — Correctness (hard requirement, target ≥ 0.95)

- **`F1_correct`** — the number that matters most. Precision + recall
  of "returned the right part for this OEM" where "right" = the top
  result's description contains ≥ 2 of the `GoodTokens` for the
  expected category (case-insensitive). Zero tolerance for
  wrong-category returns; a parts customer receiving the wrong disc is
  worse than receiving nothing.
- **`F1_hit`** — softer: just "returned any result at all". Useful as
  a floor to catch total-outage regressions.

Alert floor (workflow will open an issue when tripped):
- `F1_correct < 0.30` (absolute floor, will tighten as milestones progress)
- `F1_hit < 0.60`

### Tier 2 — Replacement richness (graded, more is better)

- **`AvgRepl_correct`** — average total replacements per correct-part
  hit. `Total = AftermarketCount + OEMNumbersCount + (1 if HasSupersession else 0)`.
- **`AvgAM_correct`** — average aftermarket-brand alternatives per
  correct-part hit. On wear parts (brake pads, oil filters, cabin
  filters) the target is ≥ 5 — a seller wants brand + price choice.
- **`AvgOEMx_correct`** — average OEM cross-references per correct
  hit. Signals how well the OEM equivalence table (`articlecrosses`)
  is populated for that category.

### Tier 3 — Richness bars (F1 at replacement thresholds)

Not the top hit alone but "top hit is correct AND has ≥ N total
replacements" — the seller-cares-about number:

- **`F1_rich3`**  — correct part + ≥ 3 total replacements
- **`F1_rich5`**  — correct part + ≥ 5 total replacements
- **`F1_rich10`** — correct part + ≥ 10 total replacements

---

## Reading a subfolder — quick recipe

1. Open `summary.txt`. It tells you the overall F1_hit / F1_correct /
   AvgRepl at a glance and calls out categories below the 0.95 floor.
2. If a category regressed, open `by-category.csv` and sort by
   `F1_correct` ascending — the top rows are the worst offenders.
3. To find *which strategy* broke, open `by-strategy.csv` and
   `by-strategy-slice.csv`. A drop concentrated on one strategy row
   points at that strategy's code path.
4. `failures.csv` has every failing OEM with a `Reason` column —
   feed those OEMs into the local audit runner (see
   `scripts/audit/README.md`) to reproduce.

---

## Graceful-degradation contract

The workflow degrades **soft**, not **hard**:

- If the qa endpoint is unreachable, the workflow logs a
  `::warning::` annotation and exits 0. It does **not** turn the
  maintainer's dashboard red because deploy availability is not a
  workflow bug.
- Individual optional steps use `continue-on-error: true`, so a
  transient failure (network hiccup during commit-back, artifact
  upload flake) never fails the whole run.
- The alert-issue step only opens a GitHub issue when the audit
  *actually ran* AND a metric floor tripped. Missing signal ≠ alert.

If you see a green run with no report subfolder for the date, look at
the run's `Health check` step for the reachability warning.

---

## Retention

Currently: **keep everything**. All prior nightly reports live on the
`nightly-audits/{date}` branches (the workflow force-pushes there) and
on `main` under this folder as the workflow merges each day's report
back.

**TODO** (tracked in [`docs/ROADMAP.md`](../../ROADMAP.md) → M6.S1
follow-ups): rotate to a 90-day rolling window — older subfolders
archived to a tarball on the `nightly-audits-archive` branch and
removed from `main` to keep repo checkout size bounded. Not yet
implemented; safe to defer until we accumulate ≥ 90 days of data.

---

## Secrets required for real runs

The workflow will only produce useful data once these are set on the
repository (**Settings → Secrets and variables → Actions**):

| Secret / var        | Used for                                            | Notes                                                        |
|---------------------|-----------------------------------------------------|--------------------------------------------------------------|
| `GITHUB_TOKEN`      | Commit results + open alert issues                  | Provided automatically by GitHub — no manual setup           |
| (endpoint override) | `workflow_dispatch` input, defaults to qa.ifritah.com | No secret needed unless you run against a private endpoint  |

The workflow does **not** require any DB credentials — it hits the
public search API (`/api/search`) on the configured endpoint, so
whatever DB access the deployed server has is what the audit sees. If
in future the audit needs to hit a private endpoint (staging behind
auth, per-tenant subdomain, etc.), add a `AUDIT_ENDPOINT` and
`AUDIT_BEARER_TOKEN` pair here.

Until the workflow has been triggered at least once successfully
against a real endpoint, this folder will contain only this README.

---

## Related

- Audit runner: [`scripts/audit/audit-quality.ps1`](../../../scripts/audit/audit-quality.ps1)
- Analyzer: [`scripts/audit/analyze-quality.ps1`](../../../scripts/audit/analyze-quality.ps1)
- Canary corpus: [`scripts/audit/corpus-canary-v1.csv`](../../../scripts/audit/corpus-canary-v1.csv)
- Full corpus: [`scripts/audit/corpus-1500-v2.csv`](../../../scripts/audit/corpus-1500-v2.csv)
- Sprint doc: [`docs/sprints/M5-M6-intelligence-and-production.md`](../../sprints/M5-M6-intelligence-and-production.md)
- Roadmap entry: [`docs/ROADMAP.md`](../../ROADMAP.md) — M6.S1.T1
- PR-gate variant (blocks merges on regression): [`.github/workflows/pr-quality-gate.yml`](../../../.github/workflows/pr-quality-gate.yml)
