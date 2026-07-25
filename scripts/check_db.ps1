$queries = @(
    "SELECT COUNT(*) as cnt FROM articles",
    "SELECT articleNumber FROM articles LIMIT 10",
    "SELECT articleNumber FROM articles WHERE articleNumber LIKE '%5293%'",
    "SELECT rawNumber FROM oem_cross_ref WHERE rawNumber LIKE '%5293%'",
    "SELECT COUNT(*) as cnt FROM oem_cross_ref"
)
foreach ($q in $queries) {
    Write-Host "SQL: $q"
    $out = & sqlite3 c:\ssda\chatGPT\parts-engine\data\hk_parts.db $q 2>&1
    Write-Host $out
    Write-Host ""
}
