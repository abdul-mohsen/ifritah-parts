# PR quality gate — operator guide

## Purpose

Every pull request that touches search-quality code (the request layer,
the SQL layer, or the audit tooling itself) must not regress the
canary-corpus `F1_correct` score on the `real_hk_seeded` slice by more
than **0.02** relative to the most recent blessed nightly baseline.

`F1_correct` is the "right-category part returned" score defined by
`scripts/audit/analyze-quality.ps1` — hit + description matches
`ExpectedCategory` tokens. It is the correctness bar the seller
prioritizes above all other metrics; a drop here means the search
started returning **wrong** parts.

Enforced by `.github/workflows/pr-quality-gate.yml`, which triggers on
PRs whose changed files include any of:

- `internal/service/**`   — request layer (search engine, ranking, enrichment)
- `db/queries/**`         — SQL layer  (sqlc-generated queries + supporting SQL)
- `scripts/audit/**`      — audit tooling (corpus, runner, comparator)

The workflow runs an audit against `qa.ifritah.com`, then runs
`scripts/audit/compare-audits.ps1` between the PR's fresh
`by-slice.csv` and the latest nightly `by-slice.csv` on the
`nightly-audits/{date}` branch. Verdict lands as a PR comment and,
when it blocks, a failing check.

## How the threshold is set

Default: **0.02** absolute drop in `F1_correct` on the target slice.

The value is set two ways:

1. **Workflow default** — the `REGRESSION_THRESHOLD` env var in
   `.github/workflows/pr-quality-gate.yml`. Change this to change the
   default for every PR.
2. **Per-run override** — for manual runs, use the
   `regression_threshold` input on `workflow_dispatch`. Also
   `slice` (default `real_hk_seeded`) and `endpoint` (default
   `https://qa.ifritah.com`) can be overridden.

Rationale for 0.02:

- The 200-OEM canary has 100 rows in `real_hk_seeded`. F1_correct is
  quantised in steps of roughly 0.01 at that N.
- Two-quantum drop (0.02) is above audit-run noise (which we've measured
  at roughly 0.005 between back-to-back runs on the same commit) and
  below "real regression" territory (which starts around 0.05).
- Empirically, 0.02 blocks obvious regressions without triggering on
  flaky-network runs.

If you find the gate false-blocking too often, raise the threshold in
the workflow env. If you find real regressions slipping through,
lower it. Don't touch `compare-audits.ps1`'s default — the workflow
is the source of truth.

## Updating the blessed baseline

The baseline is **the most recent `docs/reports/nightly/{date}/by-slice.csv`
that lives on a `nightly-audits/{date}` branch**. This is produced
exclusively by `.github/workflows/nightly-audit.yml` (M6.S1.T1).

You **do not** commit baselines by hand. To update it:

1. Wait for the next nightly cron (03:00 UTC).
2. If the previous nightly's numbers regressed, the workflow will have
   opened an issue tagged `regression`. Fix that first.
3. Once a nightly runs against a healthy `main`, its `by-slice.csv`
   automatically becomes the new baseline the next time any PR
   quality gate runs.

To force a baseline refresh (for example, after a big legitimate
improvement that you *want* to become the new floor):

```
# Trigger the nightly manually from the Actions tab (workflow_dispatch)
# or:
gh workflow run nightly-audit.yml
```

The nightly writes to a branch, not to `main`. That branch is the
source of truth for the baseline until a newer one lands.

## Emergency bypass

For truly urgent hotfixes where the quality gate is blocking a
critical bug fix:

1. Add `[skip-quality-gate]` **verbatim** to any commit message on the
   PR branch (case-sensitive, brackets included).
2. The workflow detects the marker and posts a bypass comment instead
   of blocking.
3. The marker only masks the **automated** check — merging still
   requires an approving review from a code owner listed in
   `.github/CODEOWNERS`. That is where the human-in-the-loop check
   lives; the bypass marker alone is not enough to merge.

Example commit message:

```
fix(search): emergency patch for XSS in query parser

Regression on seeded slice is expected — will be recovered in follow-up.

[skip-quality-gate]
```

After merging a bypassed PR, open a follow-up issue tagged
`quality-followup` referencing the bypass so the next nightly's
regression is not surprising.

## Reference

- Comparator script:     `scripts/audit/compare-audits.ps1`
- Comparator tests:      `scripts/audit/test-compare-audits.ps1`
- Workflow:              `.github/workflows/pr-quality-gate.yml`
- Baseline producer:     `.github/workflows/nightly-audit.yml`
- Audit runner:          `scripts/audit/audit-quality.ps1`
- Audit analyzer:        `scripts/audit/analyze-quality.ps1`
- Corpus:                `scripts/audit/corpus-200-canary.csv`
- Exit codes: `0` pass, `1` block (regression), `2` corpus mismatch (soft-fail)
