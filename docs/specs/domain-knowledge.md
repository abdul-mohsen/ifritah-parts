# Parts Search — Domain Knowledge Reference

**Repo:** `parts-engine-baseline`  
**Audience:** engineers building search strategies, QA agents writing test cases  
**Last updated:** 2026-08-16  

> Rule: Every search strategy decision in the engine must be traceable to a
> rule in this document. If a strategy uses a heuristic not listed here, add it.

---

## 1. Core Principles

### 1.1 Spec interchangeability

> Two parts are interchangeable when their **specifications match**, not only
> when a cross-reference database says so.

Database records are incomplete. TecDoc has ~60% coverage of the Western
European aftermarket. Many Korean/Japanese OEM numbers are absent. When the
database is silent, physical specifications are the fallback truth.

### 1.2 Component dependency — the most important principle

> Every significant component has sub-components attached to it. The parent
> component's physical specifications directly constrain which sub-components
> will fit. If you know the parent's specs, you can derive what its children
> must be — even when the database has no explicit link.

This is not limited to engines. It applies to every assembly in the vehicle.

**How it works:**

```
User provides:  a parent component (or its specs or its OEM number)
System deduces: the spec constraints on every child part of that assembly
System searches: for sub-components matching those derived specs
```

**Why this matters for search:**  
A user searching for a "timing belt" may not know the exact OEM number. But
if they give you the engine or the parent timing kit assembly, you know the
belt teeth count, width, and pitch from that parent's spec. You search for
those dimensions directly — the database does not need a cross-ref record.

**Why this is better than OEM lookup alone:**  
OEM lookup only finds what the database explicitly links. Assembly dependency
deduction finds anything whose physical spec fits the parent — including
aftermarket parts from brands not yet in the cross-ref tables.

---

### 1.3 Assembly dependency examples (not exhaustive)

| Parent component | Its spec that matters | Child parts constrained by it |
|------------------|-----------------------|-------------------------------|
| Engine block | Displacement, cylinders, fuel system, timing type | Spark plugs, oil filter, timing belt/chain, injectors, glow plugs, water pump, serpentine belt, crankshaft sensor |
| Suspension strut | Body diameter, eye bolt diameter, spring perch diameter, travel | Coil spring, top mount, dust boot, bump stop |
| Brake caliper | Caliper bolt pattern, piston diameter, pad slot dimensions | Brake pads, caliper bolts, caliper guide pins, piston seal kit |
| Brake rotor | Rotor diameter, hat height, center bore, PCD | Wheel spacers, hub rings, lug bolts |
| Turbocharger | Inlet flange type, outlet flange type, oil feed banjo thread | Air intake pipe, intercooler pipes, oil feed line, oil return gasket |
| Radiator | Inlet port diameter, outlet port diameter, mounting width | Upper hose, lower hose, overflow tank pipe |
| AC compressor | Drive belt groove count, flange thread | Serpentine belt (groove must match), suction/discharge line fittings |
| Transmission (manual) | Input shaft spline count, torque rating | Clutch disc spline count, flywheel bolt pattern, release bearing bore |
| Wheel hub / knuckle | Hub bore diameter, PCD, ABS ring diameter | Wheel bearing inner diameter, ABS wheel speed sensor |
| Fuel pump (in-tank) | Fuel pump module flange diameter | Fuel pump strainer, fuel pump connector, sending unit |
| Exhaust manifold | Flange bolt pattern, stud thread size | Exhaust gasket, manifold studs, downpipe flange |
| Alternator | Pulley groove count, pulley type (solid/OAD) | Serpentine belt groove count must match; OAD vs solid must match |
| Steering rack | Tie rod thread size, rack end spline | Inner tie rod, outer tie rod, boot kit |
| Differential | Ring gear bolt pattern, pinion thread | Crown wheel bolts, pinion nut, differential bearings |
| Battery | Group size (L×W×H), terminal polarity | Battery tray, hold-down clamp, terminal adapters |
| Coolant thermostat | Housing bolt pattern, outlet diameter | Thermostat gasket, housing O-ring, coolant hose (outlet side) |

---

### 1.4 Which parent→child deductions are reliable vs unreliable

Not all parent specs reliably constrain all children. Document the confidence
level per relationship.

| Relationship | Reliability | Reason |
|-------------|-------------|--------|
| Engine → spark plug thread/reach | HIGH | Standardised per engine family |
| Engine → oil filter thread | HIGH | One thread standard per engine |
| Engine → timing belt teeth count | CRITICAL — must be exact | Wrong count = engine damage |
| Engine → injector flow rate (MPI) | HIGH | Flow rate standardised per engine |
| Engine → injector flow rate (GDI) | MEDIUM | GDI varies by calibration |
| Strut body diameter → spring seat | HIGH | Physical fit; same diameter required |
| Caliper → pad slot dimensions | HIGH | Pad must slide into caliper bracket |
| Turbo inlet flange → intake pipe | HIGH | Flange bolt pattern is standardised |
| Radiator port → hose diameter | HIGH | Port outer = hose inner diameter |
| Transmission torque rating → clutch | MEDIUM | Depends on friction material choice |
| Wheel hub PCD → lug bolts | HIGH | Thread pitch and PCD are exact |
| Alternator pulley groove → belt | HIGH | Groove count must match exactly |
| Battery group size → tray | MEDIUM | Tray dimensions are approximate fits |
| Exhaust manifold flange → gasket | HIGH | Bolt pattern is exact |

**Rule:** When reliability is HIGH or CRITICAL, the engine must enforce that
spec as a hard filter. MEDIUM reliability: return results but add a
`ConfidenceNote` warning. Never silently return incompatible sub-components.

---

## 2. Deduction Chain: VIN → Part

The richest query a user can provide is a VIN. Everything derivable from it
should be extracted before falling back to text search.

```
VIN (17 chars)
  ├── WMI (chars 1–3)  → Make, country, plant
  ├── VDS (chars 4–9)  → Model, body style, engine code, check digit
  └── VIS (chars 10–17) → Model year (char 10), assembly plant, serial

Engine code (from VDS + manufacturer decode)
  ├── Family          → e.g., G4K = Hyundai Theta II
  ├── Displacement    → e.g., G4KA=2.0L, G4KC=2.4L, G4KJ=2.4L GDI
  ├── Configuration   → DOHC / SOHC, 4-cyl / 6-cyl / V
  ├── Fuel system     → MPI / GDI / CRDI (diesel) / Hybrid
  └── Timing type     → Belt or Chain (affects service parts)

From engine code → deduce:
  ├── Spark plug thread (M14×1.25 most 4-cyl; M12×1.25 smaller engines)
  ├── Spark plug reach (19mm GDI engines; 17.5mm MPI engines)
  ├── Number of spark plugs = number of cylinders
  ├── Oil filter thread (M20×1.5 most Hyundai/Kia; 3/4-16 some diesels)
  ├── Air filter type (panel vs pod — determined by airbox design per model)
  ├── Timing belt teeth count (if belt type — critical: wrong teeth = engine damage)
  ├── Number of injectors = number of cylinders (GDI) or 1 (TBI)
  └── Alternator output range (typically 90–150A for 4-cyl passenger)

From platform code → deduce:
  ├── Suspension geometry → shock absorber travel and mount type
  ├── Brake system → rotor diameter range by trim
  ├── Body dimensions → exterior panel fitment
  └── Platform-shared models → cross-brand compatible parts

From body style (from VDS or linkagetargets.bodyStyle) → deduce:
  ├── Wiper blade length (varies sedan vs SUV vs hatchback)
  ├── Headlight assembly part number range
  └── Body panel dimensions
```

### VIN decode implementation path
1. NHTSA vPIC API (`internal/nhtsa/decoder.go`) → make, model, year, engine
2. `linkagetargets` MySQL query (`01_vehicle_resolution.sql` steps 1–4) → `linkageTargetId`
3. `linkageTargetId` → `articlesvehicletrees` → all compatible parts for that vehicle

**Coverage limitation:** NHTSA covers US-market VINs. Saudi-market HK vehicles
may have different WMIs (e.g., KMHXX… for Kia Bahrain assembly). Extend the
WMI table for GCC-spec VINs as Sprint 6 task.

---

## 3. Part Categories — Fitment Rules and Spec-Based Search

For each category the table lists:
- **FitmentDriver** — what vehicle attribute determines compatibility
- **Primary match key** — the strongest signal
- **Spec dimensions** — what `articlecriteria` fields matter for interchangeability
- **Platform sharing** — whether same-platform models share this part
- **Aftermarket coverage** — how well TecDoc cross-refs cover this category
- **Known limitations** — where the database will fail and why

---

### 3.1 Spark Plug

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±300 |
| Primary match | Engine code → thread diameter + reach |
| Platform sharing | HIGH (same engine = same plug across models) |
| Aftermarket coverage | EXCELLENT (NGK, DENSO, BOSCH all publish cross-ref tables) |

**Spec dimensions that determine interchangeability:**
```
thread_diameter    M14×1.25  (most 4-cyl gasoline)
                   M12×1.25  (smaller engines, e.g., 1.0L–1.4L)
thread_reach       17.5mm    (MPI/older engines)
                   19mm      (GDI/direct injection engines — longer reach)
hex_size           16mm (most compact) | 14mm (iridium plugs)
seat_type          flat (gasket) | tapered (conical)
heat_range         5–8 (NGK scale), 1–7 (Bosch W-scale)
electrode_type     standard | platinum | iridium | double-iridium
gap                0.8mm (most HK GDI) | 1.0–1.1mm (NGK iridium)
```

**Spec-based search rule:**  
Given engine code G4KJ → `thread=M14×1.25, reach=19mm, seat=flat, gap=0.8mm`  
Query `articlecriteria` for all articles where those criteria match.  
This finds DENSO, BOSCH, CHAMPION equivalents even when `articlecrosses` is empty.

**OEM example chain:**
```
18846-09070 (HYUNDAI OEM, G4KJ 2.4L GDI)
  → NGK IFR6T11
  → DENSO SK20PR-A11
  → BOSCH ZR5TPP33
  → CHAMPION OE206/T10
```

**Limitation:** Heat range is engine/tuning specific. Colder-range plugs for
turbo builds are not interchangeable with stock. The engine must not be
modified for spec-based lookup to be safe.

---

### 3.2 Oil Filter

| Field | Value |
|-------|-------|
| FitmentDriver | `FitUniversal` — dimension-driven |
| Primary match | Thread size + bypass pressure |
| Platform sharing | HIGH (same engine = same filter across body styles) |
| Aftermarket coverage | GOOD (MANN, BOSCH, MAHLE, FEBI cover most HK engines) |

**Spec dimensions:**
```
thread_size          M20×1.5  (most Hyundai/Kia 4-cyl petrol)
                     M22×1.5  (some 6-cyl)
                     3/4-16 UNF (some diesel)
bypass_pressure      1.0–2.0 bar (must match OEM spec)
anti_drainback       yes/no (recommended for overhead cam engines)
height               varies — important for clearance
diameter             varies — important for wrench size
```

**Spec-based search rule:**  
`thread=M20×1.5, bypass_psi=14–20 (1.0–1.4 bar), anti_drainback=yes`  
→ MANN W712/4, BOSCH 0 451 103 314, MAHLE OC91, FEBI 32956 are all equivalent.

**Limitation:** `FitUniversal` classification means the engine ignores CC margin
checks. That is correct for filters — a MANN W712/4 fits a 1.6L and 2.4L if
the thread matches. But the catalog must confirm via `articlesvehicletrees`
rather than just spec, because some engines use different mounting angles.

---

### 3.3 Air Filter (Intake)

| Field | Value |
|-------|-------|
| FitmentDriver | `FitUniversal` — physical dimensions |
| Primary match | Filter element dimensions (L×W×H) OR airbox fitment code |
| Platform sharing | MEDIUM (same platform, same airbox sometimes) |
| Aftermarket coverage | GOOD |

**Spec dimensions:**
```
filter_length      mm
filter_width       mm
filter_height      mm
filter_shape       panel | cylindrical | conical
flange_diameter    mm (for pod filters)
```

**Limitation:** Airbox design is model-specific even on shared platforms. A
Hyundai Tucson and Kia Sportage (shared N platform) may use the same filter or
different ones depending on engine variant. Always verify via
`articlesvehicletrees` rather than assuming platform sharing.

---

### 3.4 Cabin Filter (Pollen Filter)

| Field | Value |
|-------|-------|
| FitmentDriver | `FitBody` — model-specific housing |
| Primary match | Body model + year range |
| Platform sharing | LOW (cabin varies per interior design) |
| Aftermarket coverage | EXCELLENT |

**Note:** Located behind the glove box or at the cowl. Access procedure varies.
The filter dimensions are secondary — housing shape drives fitment.

---

### 3.5 Timing Belt / Timing Chain

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±300 (strict) |
| Primary match | Engine code |
| Platform sharing | HIGH (same engine = same belt across models) |
| Aftermarket coverage | GOOD for European brands; POOR for Asian-specific kits |

**Spec dimensions (timing belt):**
```
teeth_count     e.g., 130T (critical — wrong count = engine damage)
width_mm        e.g., 25mm
pitch           MXL | XL | HTD (GATES/DAYCO standards)
```

**Critical rule:** Timing belt teeth count is a hard constraint. Do NOT return
belts with different teeth counts as alternatives. Use `articlecriteria.isMandatory=true`
for teeth_count filter.

**Belt vs chain:** G4KC/G4KJ (Theta II) uses a timing chain — no belt
replacement. G4KA (earlier 2.0L) uses a belt. Engine code lookup is mandatory.

**Limitation:** Aftermarket timing kits often include belt + tensioner + idler.
These are "kit" articles in TecDoc under `assemblyGroupNodeId` for timing kit.
The kit query is different from individual belt query.

---

### 3.6 Brake Pads / Brake Rotors

| Field | Value |
|-------|-------|
| FitmentDriver | `FitBrake` — trim and axle specific |
| Primary match | Linkage target (vehicle variant) + axle (front/rear) |
| Platform sharing | MEDIUM (same platform usually = same brakes, but sport trims differ) |
| Aftermarket coverage | EXCELLENT (ATE, Brembo, TRW, Zimmermann, EBC cover all HK) |

**Spec dimensions (rotor):**
```
outer_diameter_mm   e.g., 300mm, 320mm
thickness_new_mm    e.g., 28mm, 22mm (vented vs solid)
hat_height_mm       distance from hat to friction face
center_bore_mm      hub bore diameter
bolt_circle         PCD (e.g., 5×114.3)
vented              yes/no
```

**Spec dimensions (pad):**
```
height_mm
width_mm
thickness_mm
friction_code       ECE R90 friction material classification
```

**Spec-based search rule:**  
Given a brake rotor with `diameter=300mm, thickness=28mm, vented=yes, PCD=5×114.3`:  
Query `articlecriteria` for all brake rotor articles where these specs match.  
This finds ATE, Zimmermann, EBC equivalents even without a direct OEM cross-ref.

**Limitation:** Sport/performance trims (e.g., Hyundai Elantra N, Kia Stinger GT)
use larger rotors than standard. The `linkageTargetId` must distinguish between
base and sport trims — not all `linkagetargets` rows do this clearly.

---

### 3.7 Shock Absorber / Strut

| Field | Value |
|-------|-------|
| FitmentDriver | `FitBody` + axle position |
| Primary match | Axle (front/rear) + left/right + platform |
| Platform sharing | HIGH (same platform suspension geometry = same shock travel/mount) |
| Aftermarket coverage | GOOD (KYB, Monroe, Bilstein, SACHS cover HK platforms) |

**Spec dimensions:**
```
extended_length_mm    e.g., 450mm
compressed_length_mm  e.g., 300mm
piston_diameter_mm    e.g., 46mm (gas), 36mm (oil)
mounting_top          bolt pattern / pin diameter
mounting_bottom       eye bolt / fork / sleeve diameter
spring_perch_dia_mm   for struts with integrated spring perch
```

**Deduction from platform:**  
Hyundai Tucson (NX4) and Kia Sportage (NQ5) share the N3 platform.  
Front strut geometry is identical → same shock absorber.  
This must be confirmed via `platform_pairs` materialized view (Sprint 6 task).

**Limitation:** Lowering springs or sport suspension changes compressed/extended
length. Standard OEM replacement parts will not match modified specifications.
The engine has no signal for suspension modifications.

---

### 3.8 CV Axle / CV Joint

| Field | Value |
|-------|-------|
| FitmentDriver | `FitDrivetrain` — FWD/AWD/RWD specific |
| Primary match | Drivetrain type + axle side (left/right) |
| Platform sharing | MEDIUM (same platform but different drivetrain = different axle) |
| Aftermarket coverage | MODERATE (GKN, SKF, PASCAL cover common HK axles) |

**Spec dimensions:**
```
inner_spline_count   e.g., 23, 26, 28
outer_spline_count   e.g., 23, 26
shaft_diameter_mm
shaft_length_mm      (important for proper CV angle)
boot_diameter_small_mm
boot_diameter_large_mm
```

**Critical rule:** Spline count is a hard constraint. Different spline counts
are physically incompatible. Use `articlecriteria.isMandatory=true` for spline
filter. Do NOT return axles with different spline counts as alternatives.

---

### 3.9 Water Pump

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±500 |
| Primary match | Engine code |
| Platform sharing | HIGH (same engine = same pump) |
| Aftermarket coverage | GOOD (AISIN, GMB, GATES, SKF cover HK engines) |

**Spec dimensions:**
```
bolt_holes_count    e.g., 4, 6
bolt_circle_mm
inlet_diameter_mm
impeller_type       metal | plastic (plastic more common post-2010)
bearing_type        (affects noise level — premium vs budget)
```

**Special case:** Many modern engines (G4KJ, D4FB diesel) have a timing chain
that drives the water pump internally. Replacing it requires chain-off procedure.
External belt-driven pumps are simpler. The engine code determines which type.

---

### 3.10 Fuel Injector

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±300 (strict) |
| Primary match | Engine code + fuel system type (GDI vs MPI) |
| Platform sharing | HIGH (same engine = same injector across models) |
| Aftermarket coverage | POOR for GDI (most GDI injectors are OEM-only or Bosch remanufactured) |

**Spec dimensions:**
```
connector_type      EV1 | EV6 | USCAR (electrical connector)
flow_rate_cc_min    e.g., 200cc/min, 340cc/min, 500cc/min
fuel_pressure_bar   3 bar (MPI), 200–350 bar (GDI)
o_ring_size         upper and lower o-ring diameter
tip_type            single-hole | multi-hole | GDI spray pattern
```

**Critical distinction:** MPI injectors (port injection) and GDI injectors
(direct injection) are NOT interchangeable. GDI injectors operate at 20–35×
higher pressure and cannot substitute MPI injectors. The engine code and fuel
system field from VIN/`linkagetargets.fuelType` must determine which type.

**Limitation:** Aftermarket GDI injectors are rare and risk-prone. The engine
should warn when returning GDI injector alternatives that flow rates match.

---

### 3.11 Oxygen Sensor / Lambda Sensor

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±800 (loose) |
| Primary match | Position (upstream/downstream/bank) + connector type |
| Platform sharing | HIGH (same engine family = same sensor type often) |
| Aftermarket coverage | GOOD (Bosch, NTK/NGK, Denso cover most) |

**Spec dimensions:**
```
thread_size         M18×1.5 (universal standard)
wire_count          1 (unheated) | 3 (3-wire heated) | 4 (4-wire heated wide-band)
connector_type      Bosch 1-pin | Bosch 3-pin | Denso 4-pin etc.
signal_type         narrowband (0–1V) | wideband (linear λ output)
heated              yes/no
length_mm           sensor body length
```

**Spec-based search rule:**  
`thread=M18×1.5, wires=4, signal=wideband, connector=Bosch-4pin`  
→ finds all wideband O2 sensors compatible with heated mounting position.
This is especially useful because the database may list only 2–3 brands
when 10+ brands make the same physical sensor.

---

### 3.12 Alternator / Starter

| Field | Value |
|-------|-------|
| FitmentDriver | `FitEngine` — CCMargin ±500 (strict) |
| Primary match | Engine code + voltage system (12V standard) |
| Platform sharing | HIGH within engine family |
| Aftermarket coverage | MODERATE (Valeo, Bosch, Denso remanufactured) |

**Spec dimensions (alternator):**
```
output_amps      e.g., 90A, 110A, 140A, 180A
voltage          12V (standard) | 48V (mild hybrid)
pulley_type      standard | decoupler (OAD/OAP — important)
mounting_holes   position and count
```

**Critical rule:** Decoupler pulley alternators (OAD) must be replaced with
OAD-compatible alternators. Standard pulley alternators cannot substitute.
The `articlecriteria` must be checked for `pulley_type`.

---

### 3.13 Headlight / Tail Light

| Field | Value |
|-------|-------|
| FitmentDriver | `FitBody` — exact model/year/trim |
| Primary match | Model + year range + side (L/R) + light type |
| Platform sharing | VERY LOW (body panels do not cross-share) |
| Aftermarket coverage | MODERATE (Depo, TYC, Tyc/Magneti-Marelli for HK) |

**Note:** Face-lift vs pre-face-lift is critical. A 2018 Hyundai Tucson
headlight does NOT fit a 2016 Tucson if there was a mid-cycle refresh.
The `linkagetargets.beginYearMonth` / `endYearMonth` range must be respected.

**Spec dimensions:**
```
side             left | right
position         front | rear | side
style            halogen | LED | Xenon/HID
connection_type  connector pin pattern
```

**Limitation:** The engine should explicitly warn when a lighting part is
returned for a year range that spans a face-lift boundary. This requires a
face-lift calendar per model — not in TecDoc; needs manual maintenance.

---

### 3.14 Wiper Blade

| Field | Value |
|-------|-------|
| FitmentDriver | `FitBody` (but actually dimension-driven in practice) |
| Primary match | Blade length (mm) + attachment type |
| Platform sharing | MEDIUM |
| Aftermarket coverage | EXCELLENT |

**Spec dimensions:**
```
length_mm       e.g., 600mm driver, 400mm passenger
attachment_type hook (J-hook 9×3) | pin | top-lock | side-pin | bayonet
style           conventional | flat | hybrid
```

**Practical rule:** If the blade length and attachment type match, the part
is interchangeable regardless of OEM P/N. BOSCH, VALEO, TRICO, SWF all
publish cross-ref tables by blade length and attachment type.

---

### 3.15 Wheel Bearing / Hub Bearing

| Field | Value |
|-------|-------|
| FitmentDriver | `FitDrivetrain` + axle |
| Primary match | Hub bore diameter + flange bolt pattern |
| Platform sharing | HIGH within platform |
| Aftermarket coverage | GOOD (SKF, FAG, NSK, JTEKT/Koyo cover HK) |

**Spec dimensions:**
```
inner_diameter_mm   hub bore (e.g., 35mm)
outer_diameter_mm   housing bore
width_mm
flange_outer_dia    (for hub unit with flange)
bolt_holes_count
pcd_mm              pitch circle diameter
abs_ring            yes/no (must match if ABS present)
```

---

## 4. Platform Sharing Rules

Platform sharing is the most powerful deduction beyond exact VIN. When two
models share a platform, many parts are physically identical.

### Confirmed HK platform pairs (from `05_hyundai_kia_platform.sql`)
```
Hyundai Sonata (YF/LF) ↔ Kia Optima/K5 (TF/JF)    — same floorplan
Hyundai Tucson (TL/NX4) ↔ Kia Sportage (QL/NQ5)   — same platform
Hyundai Santa Fe ↔ Kia Sorento                      — large SUV platform
Hyundai Elantra (CN7) ↔ Kia K3/Forte (BD)          — compact platform
Hyundai i30 ↔ Kia Ceed                              — European-spec compact
Hyundai Genesis G70 ↔ Kia Stinger                  — RWD N3 platform
Hyundai Ioniq ↔ Kia Niro                            — hybrid/EV platform
```

### Other brand platforms (for Sprint 6 expansion)
```
Toyota Corolla ↔ Lexus IS (some gen)               — TNGA-C
Toyota Camry ↔ Lexus ES250/350                     — TNGA-K  
Nissan Altima ↔ Infiniti I35                       — CD platform
VW Golf ↔ Audi A3 ↔ Seat Leon ↔ Skoda Octavia     — MQB platform
BMW 1-Series ↔ 2-Series (some gen)                 — UKL/FAAR
Subaru BRZ ↔ Toyota GR86                           — T86 platform
```

**Platform sharing reliability by category:**

| Category | Platform share reliable? | Notes |
|----------|--------------------------|-------|
| Engine (spark plug, filter, injector) | YES — if same engine code | Engine code is the reliable key, not platform |
| Suspension (shock, spring, bearing) | YES | Suspension geometry defined by platform |
| Drivetrain (CV axle, transmission) | CONDITIONAL | FWD vs AWD breaks sharing |
| Brakes | MOSTLY | Sport trims may differ |
| Body panels, lights | NO | Each model has unique body |
| Cabin filter | SOMETIMES | Depends on HVAC design |

---

## 5. Aftermarket Coverage Strategy

### Tier priority for aftermarket search
```
Tier 1 — OE suppliers (make the same part that goes in the factory):
  BOSCH, DENSO, NGK, MANN, MAHLE, VALEO, SKF, ZF/SACHS, LUK,
  NSK, FAG/Schaeffler, ATE, Continental, AISIN, JTEKT/Koyo

Tier 2 — Quality aftermarket (match OE spec, may be cheaper):
  FEBI Bilstein, LEMFOERDER, KYB, Monroe, Bilstein, GATES, DAYCO,
  HELLA, BREMBO, EBC, TRW/ZF, ACDelco, Delphi, Spectra Premium

Tier 3 — Budget aftermarket (functional, higher variance):
  CHAMPION, FRAM, various Asian-brand OEM equivalents (GMB, MTC, etc.)
```

### Coverage gaps by category (known)
| Category | TecDoc cross-ref coverage | Best alternative source |
|----------|--------------------------|-------------------------|
| Spark plugs | EXCELLENT | NGK/DENSO publish full cross-ref |
| Oil filters | EXCELLENT | MANN/BOSCH cross-ref tables |
| Timing belts | GOOD | GATES/DAYCO have VIN-based lookup |
| Brake rotors | EXCELLENT | ATE/Zimmermann catalog complete |
| GDI injectors | POOR | OEM or Bosch Reman only |
| Body panels (HK) | POOR | Depo/TYC limited; OEM recommended |
| Sensors (GCC-spec) | MODERATE | GCC wiring may differ from EU spec |
| Hybrid/EV parts | VERY POOR | New category, TecDoc coverage sparse |

---

## 6. OEM Number Formats by Make

Normalisation rules that must be applied before any lookup:

| Make | Format example | Normalised |
|------|----------------|-----------|
| Hyundai/Kia | `26300-35505` | `2630035505` (remove dashes and spaces) |
| Toyota | `90915-YZZF2` | `90915YZZF2` |
| BMW | `11 42 7 953 129` | `11427953129` |
| Bosch (aftermarket) | `0 451 103 314` | `0451103314` |
| NGK (plug) | `BKR5EGP` | uppercase, no change |
| MANN | `W 712/4` | `W7124` (remove spaces and slashes) |
| MAHLE | `OC 91` | `OC91` |

**Current engine normalisation:** `NormalizeOEM()` in `internal/service/hk_scope.go`
handles HK format well. Does NOT handle BMW space-separated or MANN slash format.
This is a known gap for Sprint 6 non-HK expansion.

---

## 7. Confidence Scoring Guide

The engine currently uses keyword-based classification. This table documents
the intended confidence hierarchy for spec-based and platform-based results.

| Match type | Confidence | Notes |
|------------|------------|-------|
| VIN → exact vehicle → `articlesvehicletrees` hit | 0.98 | Strongest possible |
| OEM exact match in `oem_number` table | 0.95 | Known OEM→aftermarket link |
| OEM exact match in `articlecrosses` | 0.92 | Cross-ref verified |
| Same engine code (spec confirmed) | 0.90 | Engineering deduction, confirmed by spec match |
| Platform pair + spec confirmed | 0.85 | Deduction from platform sharing |
| Spec match only (no OEM cross-ref) | 0.80 | Interchangeable by dimensions |
| Same engine family (no spec check) | 0.75 | Soft deduction, needs verification |
| Supersession chain end | 0.75 | Part was replaced; current P/N is better |
| Category + CC range match | 0.70 | Broad fitment, not confirmed by spec |
| Prefix match (partial OEM) | 0.65 | Fuzzy, possible wrong variant |
| `tecdoc_keyword` (gated) | 0.65 | Garbage sentinel — flag for review |
| Online scraper (PartsOuq/dealer) | 0.60 | Unverified, useful as fallback only |

---

## 8. What the Database Cannot Tell You

These are cases where spec-based deduction and human domain knowledge
must substitute for database lookups:

1. **Modified vehicles** — lowered suspension, performance engines, custom
   brake setups. No signal available. Return standard parts and warn.

2. **Saudi/GCC-spec differences** — some HK vehicles sold in Saudi use different
   engine calibrations, emissions equipment, or cooling specifications.
   GCC-spec vehicles may have different OEM part numbers. TecDoc has EU-spec data.
   Mitigation: check OEM suffix (e.g., `26300-35505-SA` for Saudi variant).

3. **Face-lift boundaries** — TecDoc `linkagetargets.beginYearMonth` /
   `endYearMonth` is not always updated for mid-cycle refreshes. A 2017-model
   Tucson face-lift may still appear under the same `linkageTargetId` as a
   pre-face-lift 2016 model.

4. **Supersession without cross-ref** — a manufacturer may discontinue an OEM
   number and the supersession record exists but the new P/N has no aftermarket
   cross-ref yet (new model, parts not yet in TecDoc). Walk the chain and warn.

5. **Regional part number variations** — Hyundai Motor Middle East sometimes
   uses different suffixes for the same base part. The engine should strip known
   regional suffixes before lookup and add a warning to the result.

6. **Hybrid/EV unique parts** — 12V battery, 48V mild hybrid starter/generator,
   high-voltage battery modules, inverter coolant pump. These are not in TecDoc
   at meaningful coverage yet.

---

## 9. Search Strategies Derived from This Document

The following search modes are justified by the domain rules above:

| Mode | Domain basis | Section |
|------|-------------|---------|
| `exact_oem` | OEM normalisation + `oem_number` table | §6 |
| `cross_reference` | `articlecrosses` OEM→aftermarket bridge | §5 |
| `vehicle_fitment` | VIN → `linkageTargetId` → `articlesvehicletrees` | §2 |
| `spec_match` | `articlecriteria` dimensional interchangeability | §3.x |
| `platform_deduction` | Platform pairs → shared parts inference | §4 |
| `engine_deduction` | Engine code → derived spec search | §2 |
| `supersession_walk` | `replacedbyarticles` + `replacesarticles` chain | §3.5 |
| `functional_equivalent` | `legacy2generic` functional category matching | §3.x |
| `cross_brand` | Platform pair + brand-swap OEM mapping | §4 |
| `keyword_gated` | Text search with category-token gate | §3 |

Each mode is documented in `SPRINT-MASTER.md` as a task with file locations.

---

## 10. Per-Category Limitations Summary

| Category | Strategy that works | Strategy that fails | Reason |
|----------|--------------------|--------------------|--------|
| Spark plug | `spec_match` on thread+reach+gap | `keyword_gated` | Returns wrong heat range |
| Oil filter | `spec_match` on thread+bypass | `cross_reference` alone | Too few cross-refs for HK |
| Timing belt | `exact_oem` + teeth count spec | `platform_deduction` | Belt differs even same platform |
| Brake rotor | `vehicle_fitment` + spec | `engine_deduction` | Brakes are trim-not-engine specific |
| CV axle | `vehicle_fitment` + spline spec | `platform_deduction` | Drivetrain type breaks sharing |
| GDI injector | `exact_oem` only | `spec_match` | Flow rate alone insufficient; need spray pattern |
| Headlight | `vehicle_fitment` exact | `platform_deduction` | Body panels never cross-share |
| O2 sensor | `spec_match` on connector+wire count | `exact_oem` alone | Too few OEM cross-refs |
| Wiper blade | `spec_match` on length+attachment | `vehicle_fitment` | Dimension is the truth |
| Shock absorber | `platform_deduction` + travel spec | `engine_deduction` | Engine irrelevant for suspension |
