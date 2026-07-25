import sqlite3
conn = sqlite3.connect(r'c:\ssda\chatGPT\parts-engine\data\vpic.lite.db')
c = conn.cursor()

# Check WMIs with no MakeId
for wmi in ['4T1', 'KMH', '1N4', '5J6', 'MHF']:
    c.execute("""
        SELECT w.Id, w.Wmi, w.MakeId, m.Name, w.ManufacturerId
        FROM Wmi w LEFT JOIN Make m ON w.MakeId = m.Id
        WHERE w.Wmi = ?
    """, (wmi,))
    rows = c.fetchall()
    if rows:
        for r in rows:
            print(f'WMI {wmi}: Id={r[0]}, MakeId={r[2]}, Make={r[3]}, MfrId={r[4]}')
    else:
        print(f'WMI {wmi}: NOT FOUND')

    # Also check Wmi_Make for additional makes
    c.execute("""
        SELECT wm.WmiId, m.Name as Make
        FROM Wmi_Make wm
        JOIN Wmi w ON wm.WmiId = w.Id
        JOIN Make m ON wm.MakeId = m.Id
        WHERE w.Wmi = ?
    """, (wmi,))
    extras = c.fetchall()
    if extras:
        for e in extras:
            print(f'  Wmi_Make: {e[1]}')
    print()

conn.close()
