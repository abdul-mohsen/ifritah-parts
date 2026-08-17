# S0-T1 — Kill tecdoc_keyword fallback for OEM queries

**Date:** 2026-08-17
**Branch:** `sprint2-articlecrosses`
**Task ID:** S0-T1

## Task

```
Task ID:          S0-T1
Title:            Kill tecdoc_keyword fallback for OEM queries
Description:      When Search() routes to searchByOEM and returns 0 results, do
                  not fall through to searchByText (which invokes SearchByKeyword
                  and returns wrong-category garbage). searchByArticle fallback
                  is still allowed (owned catalog is safe).
Expected Outcome: OEM misses return empty response + warning. BUG-5, BUG-9, BUG-10,
                  BUG-11 unblocked. searchByText still runs for text queries.
```

## Developer Output

**Task:** S0-T1
**Files Changed:** `internal/service/smart_search.go` (Search() dispatcher, OEM branch)

**Implementation Summary:**

The SPRINT-MASTER plan references `smart_search.go` lines 917–947 (an outdated
line range from 2026-08-16); the actual `SearchByKeyword` call now lives in
`searchByText` at line 887 with strategy label `tecdoc_fulltext`. The path
that produces the buggy behavior described by BUG-1/5/9/10/11 is:

```
Search(query="26300-35505")
  → looksLikeOEMNumber() = true → searchByOEM → 0 results
  → falls through to searchByArticle → 0 results
  → falls through to searchByText(oem)      ← BUG-source
     → SearchByKeyword("26300-35505")       ← returns wrong-category garbage
```

The fix removes only the OEM→text fallback. `searchByText` continues to be
invoked directly for the `default:` (free-text) branch, and via the
`looksLikeArticleNumber` branch. `searchByArticle` (owned-catalog exact match)
remains in the OEM fallback chain because it is safe (exact-key lookup).

**Diff (Search() dispatcher, OEM branch):**

```diff
   // OEM search found nothing — try article lookup then text search
   artResp, err := s.searchByArticle(query, linkageTargetId, vehicleCC, limit)
   if err == nil && artResp.Total > 0 {
       return artResp, nil
   }
-  textResp, err := s.searchByText(query, vehicleCC, fuelType, page, limit)
-  if err == nil && textResp.Total > 0 {
-      return textResp, nil
-  }
+  resp.Warnings = append(resp.Warnings,
+      fmt.Sprintf("OEM %q not found; text-search fallback disabled to prevent wrong-category matches (S0-T1)", query))
   return resp, nil // return original OEM response with warnings
```

**Live QA gate (TODO — no network access this session):**

```
QA_BASE_URL=https://qa.ifritah.com go run ./cmd/qa_gate > docs/reports/S0-T1-qa.txt
```
Golden cases expected to flip FAIL → PASS: BUG-5, BUG-9, BUG-10, BUG-11.

**Status:** READY_FOR_REVIEW
