import sqlite3, re, json
conn = sqlite3.connect(r'c:\ssda\chatGPT\parts-engine\data\vpic.lite.db')
c = conn.cursor()

# Preload lookup tables
lookups = {}
for table in ['Make', 'Model', 'Country', 'VehicleType', 'BodyStyle', 'DriveType', 'FuelType']:
    c.execute(f"SELECT Id, Name FROM {table}")
    lookups[table] = {r[0]: r[1] for r in c.fetchall()}

# Preload elements
c.execute("SELECT Id, Code FROM Element")
elem_codes = {r[0]: r[1] for r in c.fetchall()}

year_codes = {
    'A': 2010, 'B': 2011, 'C': 2012, 'D': 2013, 'E': 2014,
    'F': 2015, 'G': 2016, 'H': 2017, 'J': 2018, 'K': 2019,
    'L': 2020, 'M': 2021, 'N': 2022, 'P': 2023, 'R': 2024,
    'S': 2025, 'T': 2026, 'V': 2027, 'W': 2028, 'X': 2029,
    'Y': 2030,
    '1': 2001, '2': 2002, '3': 2003, '4': 2004, '5': 2005,
    '6': 2006, '7': 2007, '8': 2008, '9': 2009,
}

def pattern_len(keys):
    n = 0
    i = 0
    while i < len(keys):
        if keys[i] == '[':
            end = keys.find(']', i)
            if end < 0:
                n += 1; i += 1
            else:
                n += 1; i = end + 1
        else:
            n += 1; i += 1
    return n

def match_keys(vin, keys):
    vds = vin[3:]
    regex = '^'
    i = 0
    while i < len(keys):
        ch = keys[i]
        if ch == '[':
            end = keys.find(']', i)
            if end < 0: return False
            regex += keys[i:end+1]
            i = end + 1
        elif ch == '*':
            regex += '[A-HJ-NPR-Z0-9]'
            i += 1
        else:
            regex += ch
            i += 1
    regex += '$'
    eff_len = pattern_len(keys)
    if eff_len > len(vds): return False
    return bool(re.match(regex, vds[:eff_len]))

def decode_vin(vin):
    vin = vin.upper()
    wmi = vin[:3]
    year = year_codes.get(vin[9], 0)

    # Find WMI
    c.execute("SELECT Id, MakeId FROM Wmi WHERE Wmi = ?", (wmi,))
    row = c.fetchone()
    if not row:
        return None
    wmi_id, make_id = row

    # Get make
    make = ''
    if make_id:
        make = lookups['Make'].get(make_id, '')
    if not make:
        c.execute("SELECT MakeId FROM Wmi_Make WHERE WmiId = ? LIMIT 1", (wmi_id,))
        r = c.fetchone()
        if r:
            make = lookups['Make'].get(r[0], '')

    # Find schemas
    c.execute("""
        SELECT DISTINCT VinSchemaId FROM Wmi_VinSchema
        WHERE WmiId = ? AND (YearFrom IS NULL OR YearFrom <= ?) AND (YearTo IS NULL OR YearTo >= ?)
    """, (wmi_id, year, year))
    schema_ids = [r[0] for r in c.fetchall()]
    if not schema_ids:
        return {'make': make, 'model': '', 'year': year}

    # Get patterns
    placeholders = ','.join(['?'] * len(schema_ids))
    c.execute(f"""
        SELECT p.Keys, p.ElementId, p.AttributeId, e.LookupTable
        FROM Pattern p JOIN Element e ON p.ElementId = e.Id
        WHERE p.VinSchemaId IN ({placeholders})
        AND e.Code IN ('Make','Model','BodyClass','DriveType','FuelTypePrimary','PlantCountry','VehicleType','DisplacementL','EngineCylinders')
    """, schema_ids)

    result = {'make': make, 'model': '', 'year': year}
    for keys, elem_id, attr_id, lookup_table in c.fetchall():
        if not match_keys(vin, keys):
            continue
        code = elem_codes.get(elem_id, '')
        if lookup_table and lookup_table in lookups:
            try:
                value = lookups[lookup_table].get(int(attr_id), attr_id)
            except:
                value = attr_id
        else:
            value = attr_id

        if code == 'Model' and not result.get('model'):
            result['model'] = value

    return result

# Test the failing VINs
failing_vins = [
    "5NPE34AF7FH123456",
    "KNDPN3AC0L7777777",
    "5XYP34A53GP000001",
    "KMTG34LA1LU000001",
    "JTDKN3DU5A0000001",
    "5TFJX4GN8LX000001",
    "JTJ77AHZ5H2000001",
    "19UUB2F34LA000001",
    "1N4AL3AP8EC000001",
    "JN8AT2MV5HW000001",
    "JF2SJABC5GH000001",
    "JA4AZ3A35HZ000001",
    "4A4AR3AU5FE000001",
    "1FTFW1ET5EKE00001",
    "1FM5K8D82FGA00001",
    "1FA6P8CF1L5000001",
    "1G1YY22G465000001",
    "1GKS1AEJ8FR000001",
    "1G4HP54K174000001",
    "1C6RR7LT7HS000001",
    "1B3CC5FB5AN000001",
    "1J4GA59167L000001",
    "5YJ3E1EA1LF000001",
    "WBAPH5C55BA000001",
    "5UXKR0C58E0000001",
    "WDDHF5KB1EA000001",
    "4JGDF7CE5FA000001",
    "WAUDFAFC2DN000001",
    "WA1LFAFP5EA000001",
    "3VW2B7AJ8DM000001",
    "WP0AB2A95ES000001",
    "WP1AA2A27HLA00001",
    "YV1RS592582000001",
    "SAJWA0ES7DPS00001",
    "SALGS2RE7LA000001",
    "SCBFR7ZA5FC000001",
    "ZFF67NFA1E0000001",
    "ZHWUC1ZF7HLA00001",
    "ZAM57RTA1F1000001",
    "ZARFAEBN5L7000001",
    "JF1GD67697S000001",
    "ML3AA00A0K0000001",
    "LYVAA00A0N0000001",
    "LC0AA00A0R0000001",
]

for vin in failing_vins:
    r = decode_vin(vin)
    if r and r.get('model'):
        print(f'{vin} -> model="{r["model"]}"')
    else:
        print(f'{vin} -> NO NHTSA MATCH')

conn.close()
