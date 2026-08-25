# M4.S2 Regional Supplier Catalog Survey

**Task:** M4.S2.T1 from `docs/sprints/M4-data-sources.md`.

**Goal:** identify the 5 largest regional Hyundai/Kia parts distributors serving KSA / UAE / Oman. For each, determine: catalog access mechanism, cost, freshness, integration feasibility. Recommend which 2 to build importers for in M4.S2.T2.

**Owner:** business-dev + data-sources.

## Candidate suppliers

| Supplier | Region | Type | Notes |
|---|---|---|---|
| **Ali Al-Ghanim Auto** | Kuwait / GCC | Multi-brand distributor | Long-standing HK partner; catalog reportedly available via B2B portal |
| **Al-Futtaim Motors** | UAE / wider MENA | HK exclusive distributor | Largest HK dealer network in the region; likely has GSW/KWO access |
| **Petromin** | KSA | Multi-brand + Hyundai franchise | KSA-focused; runs Petromin Nissan + Suzuki + Hyundai lines |
| **Al-Ghazlain Auto Parts** | KSA | Aftermarket wholesaler | HK-heavy aftermarket inventory; no known API |
| **Aljazirah Vehicles** | KSA | Hyundai franchise | Frontrunner HK dealer in KSA |
| **AGMC** | UAE | BMW-focused, incl. HK | Adjacent — probably not primary but worth quick check |
| **Kanoo Motors** | Bahrain / KSA | Multi-brand | Has an internal parts system; API status unknown |

## Access-mechanism per supplier (to be filled in during survey)

For each supplier, capture:

1. **Public catalog site:** URL. Does it show OEM ↔ aftermarket cross-refs?
2. **B2B portal:** exists? Login required? Data-export format (CSV / JSON / API)?
3. **Direct API partnership:** available? Contract terms? Estimated monthly cost?
4. **Data freshness:** how often is the catalog updated on the supplier side?
5. **Coverage:** HK-specific? Full aftermarket? Which categories?
6. **Contact:** name + email + phone of the technical point-of-contact.

Template block per supplier:

```
### {Supplier name}

- **Region:** ...
- **Public catalog:** {URL / "none"}
- **B2B portal:** {URL / "none"}
- **API:** {"available", "on request", or "not offered"} — {estimated cost}
- **Data freshness:** {days/weeks}
- **Coverage:** {"OEM only" | "OEM + aftermarket" | "aftermarket only"}
- **Contact:** {name / email}
- **Feasibility rating:** {1-5}
- **Notes:** ...
```

## Feasibility matrix (empty — fill during survey)

| Supplier | API? | Cost/mo | Freshness | HK coverage | Feasibility (1-5) |
|---|:-:|---:|---:|:-:|:-:|
| Ali Al-Ghanim Auto | | | | | |
| Al-Futtaim Motors | | | | | |
| Petromin | | | | | |
| Al-Ghazlain Auto Parts | | | | | |
| Aljazirah Vehicles | | | | | |
| AGMC | | | | | |
| Kanoo Motors | | | | | |

## Decision criteria for M4.S2.T2

The two suppliers picked for the M4.S2.T2 importer build must:

- **Feasibility ≥ 4/5** — realistic to integrate within one sprint.
- **HK coverage confirmed** — at least 60% of the audit corpus's real OEMs have entries.
- **Cost predictable** — flat monthly fee ≤ $500 OR published per-request pricing (avoid opaque licensing).
- **Freshness ≤ 7 days** — daily-refresh preferred; weekly acceptable.

## Deliverable format for M4.S2.T2

Each accepted supplier gets:

1. `scripts/scrapers/regional/{supplier}/main.go` — pulls their catalog. Emits NDJSON matching the shape `cmd/regional_import` expects (below).
2. `cmd/regional_import/main.go` — generic upsert into `aftermarket_regional` (schema in this PR).
3. Fifth path added to `TecDoc.FindAftermarketForOEM` for `aftermarket_regional`.
4. A test-vehicle audit run that shows `AvgAM_correct` gain ≥ 1 on at least one HK category.

## Fallback if all suppliers refuse API access

Escalation ladder:

1. Ask each about a **weekly full-catalog CSV export** we ingest manually.
2. If no CSV: **screen-scrape their B2B portal** with rotating credentials + rate limit ≥ 5s.
3. If neither: **defer M4.S2 to M4.S4** (community contributions) and note the coverage gap in the audit report.

Never build against a supplier's public catalog UI without their explicit permission; violates most B2B ToS.
