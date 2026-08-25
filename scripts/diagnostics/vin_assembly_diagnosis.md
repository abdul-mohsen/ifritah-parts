# M0.T3 — vin_assembly diagnosis

`vin_assembly` mode returned 0 hits when probed with the known-good HK
VIN `KMHDU4AD6DU100000` (Hyundai Elantra 2013). This diagnosis walks
through what we know and where the failure sits.

## What we know

From `/api/vin/decode` (POST):
```json
{
  "nhtsaRaw": {
    "make": "HYUNDAI",
    "model": "Elantra",
    "modelYear": "2013",
    "plantCountry": "SOUTH KOREA",
    "transmission": "Automatic 6-spd",
    "vehicleClass": "Midsize Cars"
  }
}
```

VIN decoding works — we get make + model + year. **But no `linkageTargetId` in the response.** `VinAssemblyStrategy.Search()` needs a linkage target to fan out to `articlelinkages`; without one it has nothing to look up.

## Where to look

### 1. `internal/service/strategy_assembly.go:VinAssemblyStrategy`

Read the Search() implementation. Two likely bugs:

- **Bug A**: strategy expects `req.OEM` to be the VIN string. If so, when we pass `mode=vin_assembly&q=KMHDU4AD6DU100000`, the router puts the VIN into `req.Query` but the strategy reads `req.OEM` → nil.
- **Bug B**: strategy reads `req.Query` correctly but never calls the VIN decoder. It expects the CALLER to have already decoded and passed a `linkageTargetId`.

Whichever it is, we need `VinAssemblyStrategy.Search()` to:
1. Check `req.Query` for VIN-shape (17 alphanumeric chars, no dashes).
2. Decode via the existing `internal/service/vin_decoder.go`.
3. Take the resolved `linkageTargetId` and query `articlelinkages`.
4. Return parts.

### 2. VIN decoder coverage

Confirm the decoder produces a `linkageTargetId` (not just NHTSA text):

```pwsh
curl -sk -X POST -H "Content-Type: application/json" `
  -d '{"vin":"KMHDU4AD6DU100000"}' `
  https://qa.ifritah.com/api/vin/decode
```

If the response is *only* `nhtsaRaw` (no `linkageTargetId`), then:
- either the decoder doesn't map NHTSA → linkage-target yet, or
- the mapping table is unpopulated on qa.

**Fix path:** `internal/service/vin_decoder.go` may need a step: after
NHTSA decoding, look up `(make, model, year, engine)` in
`modelseries → linkagetargets` on TecDoc MySQL and return the top-N
matching `linkageTargetId`s.

### 3. If linkage IDs are returned, does the strategy actually use them?

Trace: `VinAssemblyStrategy.Search()` → does it invoke `searchByVehicle`
with the decoded `linkageTargetId`? If yes and still 0 hits, the join
against `articlelinkages` is failing — see `M0.T4 vehicle_fitment`
diagnosis.

## Test corpus needed (M0.T3)

Add to `scripts/audit/corpus-vins-v1.csv`:

```csv
VIN,ExpectedMake,ExpectedModel,ExpectedYear,ExpectedTopCategory
KMHDU4AD6DU100000,Hyundai,Elantra,2013,Filter
KM8SR73E97U000000,Hyundai,Santa Fe,2007,Filter
KNAGE223795000000,Kia,Cerato,2009,Filter
KNAFE221595000000,Kia,Rio,2009,Filter
5NPE24AF5FH000000,Hyundai,Sonata,2015,Filter
```

Run:
```pwsh
pwsh scripts/audit/audit-quality.ps1 `
  -InputCorpus scripts/audit/corpus-vins-v1.csv `
  -Modes vin_assembly `
  -QueryColumn VIN `
  -EnrichmentLevel none
```

(Requires `-QueryColumn` support in the audit script — M0.T5 sub-task
that adds keyword corpus support.)

## Exit criterion

`vin_assembly` returns ≥ 30 parts across ≥ 10 categories for each of
the 5 corpus VINs.
