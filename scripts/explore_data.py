# Explore data formats for vehicle databases
import json, csv, os

data_dir = r"c:\ssda\chatGPT\parts-engine\data"

# 1. EPA FuelEconomy CSV - first few rows
print("=" * 60)
print("EPA FuelEconomy CSV")
print("=" * 60)
epa_file = os.path.join(data_dir, "vehicles.csv")
with open(epa_file, "r", encoding="utf-8", errors="replace") as f:
    reader = csv.DictReader(f)
    fields = reader.fieldnames
    print(f"Fields ({len(fields)}): {fields[:20]}...")
    print(f"  + {fields[20:40]}...")
    print(f"  + {fields[40:60]}...")
    row_count = 0
    for row in reader:
        row_count += 1
        if row_count <= 2:
            # Show key fields
            print(f"\nRow {row_count}:")
            for k in ["make", "model", "year", "displ", "cylinders", "trany", "drive", "fuelType1", "VClass", "city08", "highway08", "comb08"]:
                if k in row:
                    print(f"  {k}: {row[k]}")
    print(f"\nTotal rows: {row_count}")

# 2. open-vehicle-db
print("\n" + "=" * 60)
print("open-vehicle-db makes_and_models.json")
print("=" * 60)
ovdb_file = os.path.join(data_dir, "open-vehicle-db", "makes_and_models.json")
if os.path.exists(ovdb_file):
    with open(ovdb_file, "r") as f:
        ovdb = json.load(f)
    if isinstance(ovdb, list):
        print(f"Top-level: list with {len(ovdb)} items")
        if ovdb:
            print(f"First item keys: {list(ovdb[0].keys()) if isinstance(ovdb[0], dict) else type(ovdb[0])}")
            print(f"First item: {json.dumps(ovdb[0], indent=2)[:500]}")
    elif isinstance(ovdb, dict):
        print(f"Top-level: dict with keys: {list(ovdb.keys())[:10]}")
        first_key = list(ovdb.keys())[0]
        print(f"First key '{first_key}': {json.dumps(ovdb[first_key], indent=2)[:500]}")

# 3. open-vehicle-db styles
print("\n" + "=" * 60)
print("open-vehicle-db styles (hyundai)")
print("=" * 60)
styles_dir = os.path.join(data_dir, "open-vehicle-db", "styles")
hyundai = os.path.join(styles_dir, "hyundai.json")
if os.path.exists(hyundai):
    with open(hyundai, "r") as f:
        styles = json.load(f)
    if isinstance(styles, list):
        print(f"Hyundai: {len(styles)} styles")
        if styles:
            print(f"First style: {json.dumps(styles[0], indent=2)[:500]}")
    elif isinstance(styles, dict):
        print(f"Hyundai dict keys: {list(styles.keys())[:5]}")

# 4. arthurkao vehicle data
print("\n" + "=" * 60)
print("arthurkao vehicle_make_model.json")
print("=" * 60)
arthur_file = os.path.join(data_dir, "vehicle_make_model.json")
if os.path.exists(arthur_file):
    with open(arthur_file, "r") as f:
        arthur = json.load(f)
    if isinstance(arthur, list):
        print(f"Top-level: list with {len(arthur)} items")
        if arthur:
            print(f"First item: {json.dumps(arthur[0], indent=2)[:500]}")
            print(f"Last item: {json.dumps(arthur[-1], indent=2)[:500]}")
    elif isinstance(arthur, dict):
        print(f"Dict keys: {list(arthur.keys())[:10]}")
