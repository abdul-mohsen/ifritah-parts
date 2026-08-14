import sqlite3
conn = sqlite3.connect(r'c:\ssda\chatGPT\parts-engine\data\vpic.lite.db')
c = conn.cursor()

# 1. Element table - what fields can be decoded
print('=== KEY ELEMENTS ===')
c.execute("SELECT Id, Name, Code, LookupTable FROM Element WHERE Name IN ('Make','Model','Model Year','Body Class','Drive Type','Fuel Type - Primary','Engine Displacement (CC)','Engine Number of Cylinders','Plant Country','Trim','Series','Vehicle Type') ORDER BY Id")
for r in c.fetchall():
    print(f'  {r}')

# 2. Sample WMI for Nissan 1N4
print('\n=== WMI: 1N4 ===')
c.execute("""
    SELECT w.Id, w.Wmi, m.Name as Make, mfr.Name as Manufacturer, vt.Name as VehicleType, co.Name as Country
    FROM Wmi w
    LEFT JOIN Make m ON w.MakeId = m.Id
    LEFT JOIN Manufacturer mfr ON w.ManufacturerId = mfr.Id
    LEFT JOIN VehicleType vt ON w.VehicleTypeId = vt.Id
    LEFT JOIN Country co ON w.CountryId = co.Id
    WHERE w.Wmi LIKE '1N4%'
""")
for r in c.fetchall():
    print(f'  {r}')

# 3. VinSchemas for 1N4
print('\n=== VinSchemas for WMI 1N4 ===')
c.execute("""
    SELECT wvs.Id, wvs.WmiId, vs.Id as VinSchemaId, vs.Name, vs.sourcewmi, wvs.YearFrom, wvs.YearTo
    FROM Wmi_VinSchema wvs
    JOIN Wmi w ON wvs.WmiId = w.Id
    JOIN VinSchema vs ON wvs.VinSchemaId = vs.Id
    WHERE w.Wmi LIKE '1N4%'
    ORDER BY wvs.YearFrom
    LIMIT 30
""")
for r in c.fetchall():
    print(f'  {r}')

# 4. Sample Pattern for first 1N4 VinSchema - show how decoding works
print('\n=== Sample Patterns for 1N4 VinSchemas (Element=Model) ===')
c.execute("""
    SELECT p.Id, p.VinSchemaId, p.Keys, e.Name as ElementName, p.AttributeId, vs.Name as SchemaName
    FROM Pattern p
    JOIN Element e ON p.ElementId = e.Id
    JOIN Wmi_VinSchema wvs ON p.VinSchemaId = wvs.VinSchemaId
    JOIN Wmi w ON wvs.WmiId = w.Id
    JOIN VinSchema vs ON p.VinSchemaId = vs.Id
    WHERE w.Wmi LIKE '1N4%' AND e.Name = 'Model'
    GROUP BY p.Id
    ORDER BY p.VinSchemaId
    LIMIT 30
""")
for r in c.fetchall():
    print(f'  {r}')

# 5. What's in AttributeId for those patterns?
print('\n=== Model names from AttributeId ===')
c.execute("""
    SELECT DISTINCT p.AttributeId, mo.Name as ModelName
    FROM Pattern p
    JOIN Element e ON p.ElementId = e.Id
    JOIN Model mo ON CAST(p.AttributeId AS INTEGER) = mo.Id
    JOIN Wmi_VinSchema wvs ON p.VinSchemaId = wvs.VinSchemaId
    JOIN Wmi w ON wvs.WmiId = w.Id
    WHERE w.Wmi LIKE '1N4%' AND e.Name = 'Model'
    LIMIT 20
""")
for r in c.fetchall():
    print(f'  {r}')

# 6. Let's try to decode VIN 1N4AA6AP9HC406410 step by step
print('\n=== DECODE VIN: 1N4AA6AP9HC406410 ===')
vin = '1N4AA6AP9HC406410'
wmi = vin[0:3]  # 1N4

# Find matching VinSchema via WMI + year
year_char = vin[9]  # 'H' = 2017
print(f'WMI: {wmi}, Year char: {year_char}')

c.execute("""
    SELECT vs.Id, vs.Name, wvs.YearFrom, wvs.YearTo
    FROM Wmi_VinSchema wvs
    JOIN Wmi w ON wvs.WmiId = w.Id
    JOIN VinSchema vs ON wvs.VinSchemaId = vs.Id
    WHERE w.Wmi = ?
    AND (wvs.YearFrom <= 2017 OR wvs.YearFrom IS NULL)
    AND (wvs.YearTo >= 2017 OR wvs.YearTo IS NULL)
""", (wmi,))
schemas = c.fetchall()
print(f'Matching schemas for year 2017: {len(schemas)}')
for s in schemas:
    print(f'  SchemaId={s[0]}, Name={s[1]}, Years={s[2]}-{s[3]}')

# For each schema, try to match patterns
for s in schemas:
    schema_id = s[0]
    c.execute("""
        SELECT p.Keys, e.Name, p.AttributeId, e.LookupTable
        FROM Pattern p
        JOIN Element e ON p.ElementId = e.Id
        WHERE p.VinSchemaId = ?
        AND e.Name IN ('Make', 'Model', 'Model Year', 'Body Class', 'Plant Country', 'Trim', 'Series')
    """, (schema_id,))
    patterns = c.fetchall()
    
    # Try to match each pattern's Keys against VIN positions
    matched = {}
    for keys, elem_name, attr_id, lookup in patterns:
        # Keys format: position values like "4:A,5:A" or just "AA" at positions
        # Need to understand the Keys format
        pass
    
    # Show raw patterns to understand Keys format
    c.execute("""
        SELECT p.Keys, e.Name, p.AttributeId
        FROM Pattern p
        JOIN Element e ON p.ElementId = e.Id
        WHERE p.VinSchemaId = ?
        AND e.Name IN ('Make', 'Model')
        LIMIT 10
    """, (schema_id,))
    print(f'\n  Schema {schema_id} ({s[1]}) - Make/Model patterns:')
    for r in c.fetchall():
        print(f'    Keys={r[0]}, Element={r[1]}, AttrId={r[2]}')

conn.close()
