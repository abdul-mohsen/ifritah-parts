# Debug EPA model matching
import sqlite3

db = sqlite3.connect(r"c:\ssda\chatGPT\parts-engine\data\epa_vehicles.db")
c = db.cursor()

tests = [
    ("NISSAN", "Maxima", 2017),
    ("HYUNDAI", "Elantra", 2016),
    ("TESLA", "Model 3", 2018),
    ("TOYOTA", "Camry", 2017),
    ("HONDA", "Accord", 2018),
]

for make, model, year in tests:
    # Exact match
    c.execute("SELECT model, trany, comb_mpg, vclass FROM vehicles WHERE make=? AND model=? AND year=? LIMIT 3", (make, model, year))
    rows = c.fetchall()
    if rows:
        print(f"EXACT {make} {model} {year}: {rows[0]}")
    else:
        # Case-insensitive
        c.execute("SELECT model, trany, comb_mpg, vclass FROM vehicles WHERE make=? AND UPPER(model)=UPPER(?) AND year=? LIMIT 3", (make, model, year))
        rows = c.fetchall()
        if rows:
            print(f"ICASE {make} {model} {year}: {rows[0]}")
        else:
            # LIKE
            c.execute("SELECT model, trany, comb_mpg, vclass FROM vehicles WHERE make=? AND UPPER(model) LIKE '%' || UPPER(?) || '%' AND year=? LIMIT 3", (make, model, year))
            rows = c.fetchall()
            if rows:
                print(f"LIKE  {make} {model} {year}: {rows[0]}")
            else:
                # Show what models exist
                c.execute("SELECT DISTINCT model FROM vehicles WHERE make=? AND year=? ORDER BY model", (make, year))
                models = [r[0] for r in c.fetchall()]
                print(f"MISS  {make} {model} {year} — available models: {models[:10]}")

db.close()
