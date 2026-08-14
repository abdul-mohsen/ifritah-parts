import sqlite3
conn = sqlite3.connect(r'c:\ssda\chatGPT\parts-engine\data\vpic.lite.db')
c = conn.cursor()
c.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
tables = c.fetchall()
print('=== TABLES ===')
for t in tables:
    print(t[0])
print()
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
