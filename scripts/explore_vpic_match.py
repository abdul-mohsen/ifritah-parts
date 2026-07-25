import sqlite3, re
conn = sqlite3.connect(r'c:\ssda\chatGPT\parts-engine\data\vpic.lite.db')
c = conn.cursor()

# VIN: 1N4AA6AP9HC406410
# Pos:  123456789...
vin = '1N4AA6AP9HC406410'

# Schema 12471 is the matching schema for 2017
schema_id = 12471

# Get ALL patterns for this schema, not just Model
c.execute("""
    SELECT e.Name, e.Code, p.Keys, p.AttributeId, e.LookupTable
    FROM Pattern p
    JOIN Element e ON p.ElementId = e.Id
    WHERE p.VinSchemaId = ?
    ORDER BY e.Name
""", (schema_id,))
patterns = c.fetchall()

print(f'Total patterns for schema {schema_id}: {len(patterns)}')
print()

# Group by element
from collections import defaultdict
by_elem = defaultdict(list)
for elem_name, code, keys, attr_id, lookup in patterns:
    by_elem[elem_name].append((keys, attr_id, lookup))

for elem_name in sorted(by_elem.keys()):
    items = by_elem[elem_name]
    print(f'{elem_name} ({len(items)} patterns):')
    for keys, attr_id, lookup in items[:5]:
        print(f'  Keys={keys}, AttrId={attr_id}, Lookup={lookup}')
    if len(items) > 5:
        print(f'  ... and {len(items)-5} more')
    print()

# Now let's figure out the Keys matching algorithm
# The Keys represent VDS positions 4-8 of the VIN
# Let's try matching VIN[3:3+len(keys)] against the Keys
print('=== MATCHING ATTEMPT ===')
vds = vin[3:8]  # AA6AP (positions 4-8)
print(f'VDS (pos 4-8): {vds}')

for elem_name, code, keys, attr_id, lookup in patterns:
    # Convert Keys pattern to regex
    # Keys like 'AA6A' or '[AB]Z0C' or '*Y1A'
    regex_keys = keys.replace('*', '.')
    key_len = len(re.sub(r'\[.*?\]', 'X', regex_keys))

    # Try matching against VDS starting at pos 4
    segment = vin[3:3+key_len]
    if re.fullmatch(regex_keys, segment):
        # Look up the value
        value = attr_id
        if lookup == 'Model':
            c.execute("SELECT Name FROM Model WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'Make':
            c.execute("SELECT Name FROM Make WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'Country':
            c.execute("SELECT Name FROM Country WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'VehicleType':
            c.execute("SELECT Name FROM VehicleType WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'BodyStyle':
            c.execute("SELECT Name FROM BodyStyle WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'DriveType':
            c.execute("SELECT Name FROM DriveType WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        elif lookup == 'FuelType':
            c.execute("SELECT Name FROM FuelType WHERE Id = ?", (int(attr_id),))
            row = c.fetchone()
            if row: value = row[0]
        print(f'  MATCH: {elem_name} = {value}  (Keys={keys}, segment={segment})')

# Also check: what does VinSchema contain for position info?
print('\n=== VinSchema details ===')
c.execute("SELECT * FROM VinSchema WHERE Id = ?", (schema_id,))
row = c.fetchone()
print(f'VinSchema: {row}')

# Check Wmi_VinSchema for position info
c.execute("SELECT * FROM Wmi_VinSchema WHERE VinSchemaId = ? LIMIT 5", (schema_id,))
for r in c.fetchall():
    print(f'  Wmi_VinSchema: {r}')

conn.close()
