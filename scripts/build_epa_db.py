# Build EPA SQLite database from vehicles.csv
import sqlite3, csv, os

data_dir = r"c:\ssda\chatGPT\parts-engine\data"
csv_path = os.path.join(data_dir, "vehicles.csv")
db_path = os.path.join(data_dir, "epa_vehicles.db")

if os.path.exists(db_path):
    os.remove(db_path)

conn = sqlite3.connect(db_path)
c = conn.cursor()

c.execute("""
CREATE TABLE vehicles (
    id INTEGER PRIMARY KEY,
    make TEXT NOT NULL,
    model TEXT NOT NULL,
    year INTEGER NOT NULL,
    cylinders TEXT,
    displ TEXT,
    drive TEXT,
    fuel_type TEXT,
    trany TEXT,
    vclass TEXT,
    city_mpg INTEGER,
    highway_mpg INTEGER,
    comb_mpg INTEGER,
    co2_gpm REAL,
    eng_descr TEXT
)
""")

count = 0
with open(csv_path, "r", encoding="utf-8", errors="replace") as f:
    reader = csv.DictReader(f)
    for row in reader:
        try:
            year = int(row.get("year", 0))
        except:
            continue
        make = (row.get("make") or "").strip().upper()
        model = (row.get("model") or "").strip()
        if not make or not model or year == 0:
            continue

        try:
            city = int(row.get("city08", 0))
        except:
            city = 0
        try:
            hwy = int(row.get("highway08", 0))
        except:
            hwy = 0
        try:
            comb = int(row.get("comb08", 0))
        except:
            comb = 0
        try:
            co2 = float(row.get("co2TailpipeGpm", 0))
        except:
            co2 = 0

        c.execute("""INSERT INTO vehicles
            (make, model, year, cylinders, displ, drive, fuel_type, trany, vclass,
             city_mpg, highway_mpg, comb_mpg, co2_gpm, eng_descr)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (make, model, year,
             row.get("cylinders", ""),
             row.get("displ", ""),
             row.get("drive", ""),
             row.get("fuelType1", ""),
             row.get("trany", ""),
             row.get("VClass", ""),
             city, hwy, comb, co2,
             row.get("eng_dscr", "")))
        count += 1

# Create indexes for fast lookups
c.execute("CREATE INDEX idx_make_model_year ON vehicles (make, model, year)")
c.execute("CREATE INDEX idx_make_year ON vehicles (make, year)")

conn.commit()

# Stats
c.execute("SELECT COUNT(*) FROM vehicles")
total = c.fetchone()[0]
c.execute("SELECT COUNT(DISTINCT make) FROM vehicles")
makes = c.fetchone()[0]
c.execute("SELECT MIN(year), MAX(year) FROM vehicles")
yr = c.fetchone()

conn.close()

size_mb = os.path.getsize(db_path) / (1024*1024)
print(f"EPA SQLite built: {total} rows, {makes} makes, years {yr[0]}-{yr[1]}, {size_mb:.1f} MB")
