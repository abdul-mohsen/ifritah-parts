# Explore the NHTSA vPIC SQLite database schema
$dbFile = "c:\ssda\chatGPT\parts-engine\data\vpic.lite.db"

Add-Type -Path "$env:USERPROFILE\.nuget\packages\system.data.sqlite.core\1.0.118.0\lib\netstandard2.1\System.Data.SQLite.dll" 2>$null

# Use Python to query schema since it has sqlite3 built in
python -c @"
import sqlite3
conn = sqlite3.connect(r'$dbFile')
c = conn.cursor()
# List all tables
c.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
tables = c.fetchall()
print('=== TABLES ===')
for t in tables:
    print(t[0])
print()
# Show schema for each table
for t in tables:
    c.execute(f'PRAGMA table_info({t[0]})')
    cols = c.fetchall()
    print(f'--- {t[0]} ---')
    for col in cols:
        print(f'  {col[1]} ({col[2]})')
    c.execute(f'SELECT COUNT(*) FROM {t[0]}')
    cnt = c.fetchone()[0]
    print(f'  Rows: {cnt}')
    print()
conn.close()
"@
