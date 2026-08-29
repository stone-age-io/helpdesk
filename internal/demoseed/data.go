package demoseed

import "time"

// Fixture vocabulary for the demo seed.
//
// Two tiers, deliberately:
//
//   - CURATED records (customers, staff, locations, things, hero tickets) are
//     hand-written. They are what you actually point at in a demo — a real
//     comment thread, a believable dispatch note, metadata that means something.
//
//   - GENERATED bulk (the rest of the tickets) is assembled from the phrase
//     tables at the bottom by a seeded PRNG. It exists to give the queue, the
//     filters, and the reports enough volume to look like a working desk rather
//     than a fixture. It is deterministic: same seed, same rows, every run.
//
// Everything here is `.example` — a reserved TLD that cannot receive mail — and
// every save is additionally marked notifications.Suppress. Belt and braces,
// because seeding a demo host that happens to have SMTP configured should not
// mail 150 strangers.

const DemoPassword = "demo12345"

type customerFixture struct {
	Key, Name, Domain, PlatformOrg string
	ShowTime                       bool
}

var customers = []customerFixture{
	{Key: "northwind", Name: "Northwind Traders", Domain: "northwind.example", PlatformOrg: "org_northwind", ShowTime: true},
	{Key: "harborview", Name: "Harborview Medical", Domain: "harborview.example"},
	{Key: "lakeside", Name: "Lakeside Schools", Domain: "lakeside.example", ShowTime: true},
	{Key: "ironbridge", Name: "Ironbridge Manufacturing", Domain: "ironbridge.example", PlatformOrg: "org_ironbridge"},
	{Key: "cedarpoint", Name: "Cedar Point Logistics", Domain: "cedarpoint.example", ShowTime: true},
	{Key: "summit", Name: "Summit Property Group", Domain: "summit.example"},
	{Key: "galewind", Name: "Galewind Energy", Domain: "galewind.example", PlatformOrg: "org_galewind"},
	{Key: "brightpath", Name: "Brightpath Clinics", Domain: "brightpath.example", ShowTime: true},
}

type staffFixture struct {
	Key, Email, Name, Role string
}

var staffMembers = []staffFixture{
	{Key: "maya", Email: "maya@816tech.example", Name: "Maya Alvarez", Role: "admin"},
	{Key: "diego", Email: "diego@816tech.example", Name: "Diego Santos", Role: "agent"},
	{Key: "priya", Email: "priya@816tech.example", Name: "Priya Nair", Role: "agent"},
	{Key: "sam", Email: "sam@816tech.example", Name: "Sam Okafor", Role: "field"},
	{Key: "tomas", Email: "tomas@816tech.example", Name: "Tomas Brandt", Role: "field"},
	{Key: "iris", Email: "iris@816tech.example", Name: "Iris Kowalski", Role: "agent"},
	{Key: "hank", Email: "hank@816tech.example", Name: "Hank Delgado", Role: "field"},
	{Key: "noor", Email: "noor@816tech.example", Name: "Noor Haddad", Role: "agent"},
	{Key: "vic", Email: "vic@816tech.example", Name: "Vic Ashford", Role: "field"},
	{Key: "gwen", Email: "gwen@816tech.example", Name: "Gwen Mbeki", Role: "admin"},
}

type requesterFixture struct {
	Key, Email, Name, Customer, Phone string
}

var requesters = []requesterFixture{
	{Key: "regina", Email: "regina.holt@northwind.example", Name: "Regina Holt", Customer: "northwind", Phone: "614-555-0142"},
	{Key: "marcus", Email: "marcus.webb@northwind.example", Name: "Marcus Webb", Customer: "northwind", Phone: "614-555-0177"},
	{Key: "sofia", Email: "sofia.reyes@northwind.example", Name: "Sofia Reyes", Customer: "northwind"},
	{Key: "anita", Email: "anita.rao@harborview.example", Name: "Anita Rao", Customer: "harborview", Phone: "216-555-0163"},
	{Key: "joel", Email: "joel.mercer@harborview.example", Name: "Joel Mercer", Customer: "harborview"},
	{Key: "petra", Email: "petra.lindqvist@harborview.example", Name: "Petra Lindqvist", Customer: "harborview"},
	{Key: "bev", Email: "bev.tanaka@lakeside.example", Name: "Bev Tanaka", Customer: "lakeside", Phone: "419-555-0188"},
	{Key: "curtis", Email: "curtis.yang@lakeside.example", Name: "Curtis Yang", Customer: "lakeside"},
	{Key: "denise", Email: "denise.okonjo@lakeside.example", Name: "Denise Okonjo", Customer: "lakeside"},
	{Key: "dale", Email: "dale.ferris@ironbridge.example", Name: "Dale Ferris", Customer: "ironbridge", Phone: "330-555-0110"},
	{Key: "nadia", Email: "nadia.brooks@ironbridge.example", Name: "Nadia Brooks", Customer: "ironbridge"},
	{Key: "emil", Email: "emil.vasquez@ironbridge.example", Name: "Emil Vasquez", Customer: "ironbridge"},
	{Key: "owen", Email: "owen.pratt@cedarpoint.example", Name: "Owen Pratt", Customer: "cedarpoint", Phone: "419-555-0121"},
	{Key: "tessa", Email: "tessa.lund@cedarpoint.example", Name: "Tessa Lund", Customer: "cedarpoint"},
	{Key: "harold", Email: "harold.kim@cedarpoint.example", Name: "Harold Kim", Customer: "cedarpoint"},
	{Key: "lena", Email: "lena.poole@summit.example", Name: "Lena Poole", Customer: "summit", Phone: "330-555-0134"},
	{Key: "raul", Email: "raul.ibarra@summit.example", Name: "Raul Ibarra", Customer: "summit"},
	{Key: "gia", Email: "gia.moretti@summit.example", Name: "Gia Moretti", Customer: "summit"},
	{Key: "bram", Email: "bram.velez@galewind.example", Name: "Bram Velez", Customer: "galewind", Phone: "216-555-0199"},
	{Key: "yuki", Email: "yuki.tanaka@galewind.example", Name: "Yuki Tanaka", Customer: "galewind"},
	{Key: "orla", Email: "orla.byrne@galewind.example", Name: "Orla Byrne", Customer: "galewind"},
	{Key: "silas", Email: "silas.moore@brightpath.example", Name: "Silas Moore", Customer: "brightpath", Phone: "614-555-0155"},
	{Key: "june", Email: "june.abara@brightpath.example", Name: "June Abara", Customer: "brightpath"},
	{Key: "wes", Email: "wes.fontaine@brightpath.example", Name: "Wes Fontaine", Customer: "brightpath"},
}

type typeFixture struct {
	Customer, Code, Name, Description string
	Schema                            map[string]any
}

// A JSON Schema shorthand: the seeder only ever writes flat object schemas.
func objSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(title string) map[string]any   { return map[string]any{"type": "string", "title": title} }
func num(title string) map[string]any   { return map[string]any{"type": "integer", "title": title} }
func boolF(title string) map[string]any { return map[string]any{"type": "boolean", "title": title} }
func date(title string) map[string]any {
	return map[string]any{"type": "string", "format": "date", "title": title}
}
func enum(title string, vals ...string) map[string]any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return map[string]any{"type": "string", "title": title, "enum": out}
}

var locationTypes = []typeFixture{
	{Customer: "northwind", Code: "warehouse", Name: "Warehouse", Description: "Distribution and storage facility.",
		Schema: objSchema(map[string]any{
			"dock_doors":         num("Dock doors"),
			"sqft":               num("Square feet"),
			"after_hours_access": boolF("After-hours access"),
		})},
	{Customer: "northwind", Code: "office", Name: "Office"},
	{Customer: "ironbridge", Code: "plant", Name: "Plant", Description: "Production facility.",
		Schema: objSchema(map[string]any{
			"shift_pattern": enum("Shift pattern", "1x8", "2x8", "3x8"),
			"hazmat":        boolF("Hazmat on site"),
		})},
	{Customer: "ironbridge", Code: "building", Name: "Building"},
	{Customer: "harborview", Code: "clinic", Name: "Clinic"},
	{Customer: "lakeside", Code: "campus", Name: "Campus"},
	{Customer: "lakeside", Code: "building", Name: "Building"},
	{Customer: "cedarpoint", Code: "yard", Name: "Yard"},
	{Customer: "summit", Code: "tower", Name: "Tower"},
	{Customer: "galewind", Code: "substation", Name: "Substation", Description: "Grid-connected switching site.",
		Schema: objSchema(map[string]any{
			"voltage_kv":     num("Voltage (kV)"),
			"manned":         boolF("Manned"),
			"last_inspected": date("Last inspected"),
		})},
	{Customer: "brightpath", Code: "clinic", Name: "Clinic"},
}

var thingTypes = []typeFixture{
	{Customer: "northwind", Code: "door-controller", Name: "Door Controller",
		Description: "Access-control panel driving one or more readers.",
		Schema: objSchema(map[string]any{
			"serial":       str("Serial number"),
			"firmware":     str("Firmware version"),
			"reader_count": num("Readers attached"),
			"poe":          boolF("PoE powered"),
		}, "serial")},
	{Customer: "northwind", Code: "kiosk", Name: "Kiosk",
		Schema: objSchema(map[string]any{
			"serial":      str("Serial number"),
			"screen_size": str("Screen size"),
			"last_imaged": date("Last imaged"),
		})},
	{Customer: "northwind", Code: "switch", Name: "Network Switch"},
	{Customer: "ironbridge", Code: "plc", Name: "PLC", Description: "Programmable logic controller on the plant floor.",
		Schema: objSchema(map[string]any{
			"serial":        str("Serial number"),
			"rack_position": str("Rack position"),
			"firmware":      str("Firmware version"),
		}, "serial")},
	{Customer: "ironbridge", Code: "sensor", Name: "Sensor",
		Schema: objSchema(map[string]any{
			"serial":        str("Serial number"),
			"measures":      enum("Measures", "vibration", "temperature", "current", "pressure"),
			"calibrated_on": date("Calibrated on"),
		})},
	{Customer: "harborview", Code: "badge-reader", Name: "Badge Reader"},
	{Customer: "harborview", Code: "nurse-call", Name: "Nurse Call Panel"},
	{Customer: "lakeside", Code: "projector", Name: "Projector"},
	{Customer: "lakeside", Code: "ap", Name: "Wireless AP"},
	{Customer: "cedarpoint", Code: "camera", Name: "Camera"},
	{Customer: "cedarpoint", Code: "gate", Name: "Gate Controller"},
	{Customer: "summit", Code: "hvac", Name: "HVAC Controller"},
	{Customer: "galewind", Code: "rtu", Name: "RTU", Description: "Remote terminal unit reporting to SCADA.",
		Schema: objSchema(map[string]any{
			"serial":   str("Serial number"),
			"protocol": enum("Protocol", "dnp3", "modbus", "iec-104"),
			"firmware": str("Firmware version"),
		}, "serial")},
	{Customer: "galewind", Code: "meter", Name: "Revenue Meter"},
	{Customer: "brightpath", Code: "badge-reader", Name: "Badge Reader"},
	{Customer: "brightpath", Code: "printer", Name: "Label Printer"},
}

type locationFixture struct {
	Customer, Code, Name, Type, Parent string
	Address, Notes, Contact, Phone     string
	Lat, Lng                           float64
	Metadata                           map[string]any
}

var locations = []locationFixture{
	{Customer: "northwind", Code: "NW-HQ", Name: "Northwind HQ", Type: "office",
		Address: "1400 Commerce Way, Columbus, OH 43215", Contact: "Regina Holt", Phone: "614-555-0142",
		Notes: "Reception badges visitors in. Dock is around the north side.", Lat: 39.9612, Lng: -82.9988},
	{Customer: "northwind", Code: "NW-HQ-2", Name: "HQ — Second Floor", Parent: "NW-HQ", Type: "office",
		Notes: "Stairwell B is the fastest way up with a cart."},
	{Customer: "northwind", Code: "NW-HQ-3", Name: "HQ — Third Floor", Parent: "NW-HQ", Type: "office"},
	{Customer: "northwind", Code: "NW-DC1", Name: "Grove City DC", Type: "warehouse",
		Address: "3900 Southwest Blvd, Grove City, OH 43123", Contact: "Marcus Webb", Phone: "614-555-0177",
		Notes: "Gate code 4417. Ask for the shift lead at the guard shack.", Lat: 39.8815, Lng: -83.0929,
		Metadata: map[string]any{"dock_doors": 24, "sqft": 180000, "after_hours_access": true}},
	{Customer: "northwind", Code: "NW-DC2", Name: "Obetz Overflow DC", Type: "warehouse",
		Address: "5100 Alum Creek Dr, Obetz, OH 43207", Lat: 39.8548, Lng: -82.9502,
		Metadata: map[string]any{"dock_doors": 8, "sqft": 60000, "after_hours_access": false}},

	{Customer: "ironbridge", Code: "IB-PLANT1", Name: "Ironbridge Plant 1", Type: "plant",
		Address: "77 Foundry Rd, Youngstown, OH 44502", Contact: "Dale Ferris", Phone: "330-555-0110",
		Notes: "Hearing protection and hi-vis required past the blue line.", Lat: 41.0998, Lng: -80.6495,
		Metadata: map[string]any{"shift_pattern": "3x8", "hazmat": true}},
	{Customer: "ironbridge", Code: "IB-PLANT1-L3", Name: "Plant 1 — Line 3", Parent: "IB-PLANT1", Type: "building",
		Notes: "Line 3 is the far bay. Lockout/tagout board is by the entrance."},
	{Customer: "ironbridge", Code: "IB-PLANT1-L4", Name: "Plant 1 — Line 4", Parent: "IB-PLANT1", Type: "building"},
	{Customer: "ironbridge", Code: "IB-PLANT2", Name: "Ironbridge Plant 2", Type: "plant",
		Address: "12 Millrace Ave, Warren, OH 44483", Lat: 41.2376, Lng: -80.8184,
		Metadata: map[string]any{"shift_pattern": "2x8", "hazmat": false}},

	{Customer: "harborview", Code: "HV-MAIN", Name: "Harborview Main Campus", Type: "clinic",
		Address: "620 Marina Dr, Cleveland, OH 44114", Contact: "Dr. Anita Rao", Phone: "216-555-0163",
		Notes: "Badge in at the service entrance. No work in patient areas before 09:00.", Lat: 41.5074, Lng: -81.6944},
	{Customer: "harborview", Code: "HV-MAIN-ER", Name: "Main — Emergency Dept", Parent: "HV-MAIN", Type: "clinic",
		Notes: "Clinical sign-off required before any outage. No exceptions."},
	{Customer: "harborview", Code: "HV-WEST", Name: "Westside Annex", Type: "clinic",
		Address: "1290 Lorain Ave, Cleveland, OH 44113", Lat: 41.4839, Lng: -81.7085},

	{Customer: "lakeside", Code: "LS-CENTRAL", Name: "Lakeside Central Campus", Type: "campus",
		Address: "15 Academy St, Sandusky, OH 44870", Contact: "Bev Tanaka", Phone: "419-555-0188",
		Lat: 41.4489, Lng: -82.7080},
	{Customer: "lakeside", Code: "LS-CENTRAL-HS", Name: "Central High School", Parent: "LS-CENTRAL", Type: "building",
		Notes: "Summer hours only in July. Check in at the main office."},
	{Customer: "lakeside", Code: "LS-CENTRAL-MS", Name: "Central Middle School", Parent: "LS-CENTRAL", Type: "building"},
	{Customer: "lakeside", Code: "LS-NORTH", Name: "North Campus", Type: "campus",
		Address: "800 Perkins Ave, Sandusky, OH 44870", Lat: 41.4310, Lng: -82.6899},

	{Customer: "cedarpoint", Code: "CP-YARD", Name: "Cedar Point Yard", Type: "yard",
		Address: "900 Rail St, Toledo, OH 43604", Contact: "Owen Pratt", Phone: "419-555-0121",
		Lat: 41.6528, Lng: -83.5379},
	{Customer: "cedarpoint", Code: "CP-XDOCK", Name: "Cross-dock Facility", Type: "yard",
		Address: "220 Matzinger Rd, Toledo, OH 43612", Lat: 41.6870, Lng: -83.5430},

	{Customer: "summit", Code: "SU-TOWER", Name: "Summit Tower", Type: "tower",
		Address: "200 High St, Akron, OH 44308", Contact: "Lena Poole", Phone: "330-555-0134",
		Notes: "Freight elevator needs a fob from building security.", Lat: 41.0814, Lng: -81.5190},
	{Customer: "summit", Code: "SU-TOWER-14", Name: "Tower — Floor 14", Parent: "SU-TOWER", Type: "tower"},
	{Customer: "summit", Code: "SU-RIVERSIDE", Name: "Riverside Commons", Type: "tower",
		Address: "45 Furnace St, Akron, OH 44308", Lat: 41.0870, Lng: -81.5170},

	{Customer: "galewind", Code: "GW-SUB-A", Name: "Substation Alpha", Type: "substation",
		Address: "4400 Lakefront Rd, Lorain, OH 44052", Contact: "Bram Velez", Phone: "216-555-0199",
		Notes: "Arc-flash PPE mandatory. Two-person rule inside the fence.", Lat: 41.4528, Lng: -82.1824,
		Metadata: map[string]any{"voltage_kv": 138, "manned": false, "last_inspected": "2026-05-02"}},
	{Customer: "galewind", Code: "GW-SUB-B", Name: "Substation Bravo", Type: "substation",
		Address: "70 Ridge Rd, Elyria, OH 44035", Lat: 41.3683, Lng: -82.1077,
		Metadata: map[string]any{"voltage_kv": 69, "manned": true, "last_inspected": "2026-03-18"}},
	{Customer: "galewind", Code: "GW-OPS", Name: "Galewind Ops Center", Type: "substation",
		Address: "1 Harbor Plaza, Lorain, OH 44052", Lat: 41.4642, Lng: -82.1799},

	{Customer: "brightpath", Code: "BP-EAST", Name: "Brightpath East Clinic", Type: "clinic",
		Address: "310 Broad St, Columbus, OH 43215", Contact: "Silas Moore", Phone: "614-555-0155",
		Lat: 39.9625, Lng: -82.9885},
	{Customer: "brightpath", Code: "BP-WEST", Name: "Brightpath West Clinic", Type: "clinic",
		Address: "2020 Hilliard Rome Rd, Columbus, OH 43026", Lat: 39.9420, Lng: -83.1360},
}

type thingFixture struct {
	Customer, Code, Name, Type, Location, Notes string
	Retired                                     bool
	Metadata                                    map[string]any
}

var things = []thingFixture{
	{Customer: "northwind", Code: "RDR-01", Name: "North Door Reader", Type: "door-controller", Location: "NW-HQ",
		Notes:    "Main visitor entrance. Fails to the locked state.",
		Metadata: map[string]any{"serial": "SN-DC-9931", "firmware": "2.4.1", "reader_count": 2, "poe": true}},
	{Customer: "northwind", Code: "RDR-02", Name: "Dock Reader", Type: "door-controller", Location: "NW-DC1",
		Metadata: map[string]any{"serial": "SN-DC-9942", "firmware": "2.3.8", "reader_count": 1, "poe": true}},
	{Customer: "northwind", Code: "RDR-03", Name: "Overflow DC Reader", Type: "door-controller", Location: "NW-DC2",
		Metadata: map[string]any{"serial": "SN-DC-9958", "firmware": "2.4.1", "reader_count": 1, "poe": false}},
	{Customer: "northwind", Code: "KSK-LOBBY", Name: "Lobby Check-in Kiosk", Type: "kiosk", Location: "NW-HQ",
		Metadata: map[string]any{"serial": "SN-KSK-2201", "screen_size": "21.5in"}},
	{Customer: "northwind", Code: "KSK-DOCK", Name: "Dock Driver Kiosk", Type: "kiosk", Location: "NW-DC1",
		Metadata: map[string]any{"serial": "SN-KSK-2214", "screen_size": "15.6in"}},
	{Customer: "northwind", Code: "SW-CORE-1", Name: "HQ Core Switch", Type: "switch", Location: "NW-HQ-2",
		Notes: "Second-floor IDF, rack 2."},
	{Customer: "northwind", Code: "SW-DC-1", Name: "DC Distribution Switch", Type: "switch", Location: "NW-DC1"},
	{Customer: "northwind", Code: "KSK-OLD", Name: "Retired Break Room Kiosk", Type: "kiosk", Location: "NW-HQ",
		Retired: true, Notes: "Decommissioned; kept for ticket history.",
		Metadata: map[string]any{"serial": "SN-KSK-1104"}},

	{Customer: "ironbridge", Code: "PUMP-7", Name: "Line 3 Feed Pump Controller", Type: "plc", Location: "IB-PLANT1-L3",
		Notes:    "Drives the feed pump on line 3. Overcurrent trips page the MSP.",
		Metadata: map[string]any{"serial": "SN-PLC-7007", "rack_position": "R2-S4", "firmware": "5.1.0"}},
	{Customer: "ironbridge", Code: "PUMP-8", Name: "Line 4 Feed Pump Controller", Type: "plc", Location: "IB-PLANT1-L4",
		Metadata: map[string]any{"serial": "SN-PLC-7012", "rack_position": "R3-S1", "firmware": "5.0.4"}},
	{Customer: "ironbridge", Code: "VIB-3A", Name: "Line 3 Vibration Sensor A", Type: "sensor", Location: "IB-PLANT1-L3",
		Metadata: map[string]any{"serial": "SN-VIB-3311", "measures": "vibration"}},
	{Customer: "ironbridge", Code: "VIB-3B", Name: "Line 3 Vibration Sensor B", Type: "sensor", Location: "IB-PLANT1-L3",
		Metadata: map[string]any{"serial": "SN-VIB-3312", "measures": "vibration"}},
	{Customer: "ironbridge", Code: "TEMP-1", Name: "Plant 1 Ambient Temp Sensor", Type: "sensor", Location: "IB-PLANT1",
		Metadata: map[string]any{"serial": "SN-TMP-4410", "measures": "temperature"}},
	{Customer: "ironbridge", Code: "PLC-P2", Name: "Plant 2 Main Controller", Type: "plc", Location: "IB-PLANT2",
		Metadata: map[string]any{"serial": "SN-PLC-8100", "rack_position": "R1-S1", "firmware": "5.1.0"}},

	{Customer: "harborview", Code: "HV-RDR-ER", Name: "ER Entrance Badge Reader", Type: "badge-reader", Location: "HV-MAIN-ER",
		Notes: "Life-safety adjacent — never take offline without clinical sign-off."},
	{Customer: "harborview", Code: "HV-RDR-MAIN", Name: "Main Lobby Badge Reader", Type: "badge-reader", Location: "HV-MAIN"},
	{Customer: "harborview", Code: "HV-RDR-W", Name: "Westside Badge Reader", Type: "badge-reader", Location: "HV-WEST"},
	{Customer: "harborview", Code: "HV-NC-3", Name: "Third Floor Nurse Call Panel", Type: "nurse-call", Location: "HV-MAIN"},

	{Customer: "lakeside", Code: "PRJ-HS-204", Name: "Room 204 Projector", Type: "projector", Location: "LS-CENTRAL-HS"},
	{Customer: "lakeside", Code: "PRJ-HS-118", Name: "Room 118 Projector", Type: "projector", Location: "LS-CENTRAL-HS"},
	{Customer: "lakeside", Code: "PRJ-MS-02", Name: "Middle School Library Projector", Type: "projector", Location: "LS-CENTRAL-MS"},
	{Customer: "lakeside", Code: "AP-HS-2F", Name: "HS Second Floor AP", Type: "ap", Location: "LS-CENTRAL-HS"},
	{Customer: "lakeside", Code: "AP-HS-GYM", Name: "HS Gymnasium AP", Type: "ap", Location: "LS-CENTRAL-HS"},
	{Customer: "lakeside", Code: "AP-N-01", Name: "North Campus AP 1", Type: "ap", Location: "LS-NORTH"},

	{Customer: "cedarpoint", Code: "CAM-NE", Name: "Northeast Yard Camera", Type: "camera", Location: "CP-YARD"},
	{Customer: "cedarpoint", Code: "CAM-SW", Name: "Southwest Yard Camera", Type: "camera", Location: "CP-YARD"},
	{Customer: "cedarpoint", Code: "GATE-MAIN", Name: "Main Gate Controller", Type: "gate", Location: "CP-YARD"},
	{Customer: "cedarpoint", Code: "GATE-XD", Name: "Cross-dock Gate Controller", Type: "gate", Location: "CP-XDOCK"},

	{Customer: "summit", Code: "HVAC-14", Name: "Floor 14 HVAC Controller", Type: "hvac", Location: "SU-TOWER-14"},
	{Customer: "summit", Code: "HVAC-RS", Name: "Riverside HVAC Controller", Type: "hvac", Location: "SU-RIVERSIDE"},
	// No code: gear the platform never onboarded. This is the case that makes
	// the catalog a superset rather than a copy, so the demo must show one.
	{Customer: "summit", Name: "Lobby Reception Printer", Location: "SU-TOWER",
		Notes: "Customer-owned, never onboarded to the platform. No code, by design."},

	{Customer: "galewind", Code: "RTU-A1", Name: "Alpha RTU 1", Type: "rtu", Location: "GW-SUB-A",
		Metadata: map[string]any{"serial": "SN-RTU-5501", "protocol": "dnp3", "firmware": "3.2.0"}},
	{Customer: "galewind", Code: "RTU-A2", Name: "Alpha RTU 2", Type: "rtu", Location: "GW-SUB-A",
		Metadata: map[string]any{"serial": "SN-RTU-5502", "protocol": "dnp3", "firmware": "3.1.7"}},
	{Customer: "galewind", Code: "RTU-B1", Name: "Bravo RTU 1", Type: "rtu", Location: "GW-SUB-B",
		Metadata: map[string]any{"serial": "SN-RTU-5610", "protocol": "modbus", "firmware": "3.2.0"}},
	{Customer: "galewind", Code: "MTR-A", Name: "Alpha Revenue Meter", Type: "meter", Location: "GW-SUB-A"},
	{Customer: "galewind", Code: "MTR-B", Name: "Bravo Revenue Meter", Type: "meter", Location: "GW-SUB-B"},

	{Customer: "brightpath", Code: "BP-RDR-E", Name: "East Clinic Badge Reader", Type: "badge-reader", Location: "BP-EAST"},
	{Customer: "brightpath", Code: "BP-RDR-W", Name: "West Clinic Badge Reader", Type: "badge-reader", Location: "BP-WEST"},
	{Customer: "brightpath", Code: "BP-PRN-E", Name: "East Clinic Label Printer", Type: "printer", Location: "BP-EAST"},
	{Customer: "brightpath", Code: "BP-PRN-W", Name: "West Clinic Label Printer", Type: "printer", Location: "BP-WEST"},
}

type projectFixture struct {
	Key, Customer, Location, Title, Description, Status, Lead string
	StartDays, TargetDays                                     int
}

var projects = []projectFixture{
	{Key: "nw-dc-access", Customer: "northwind", Location: "NW-DC1",
		Title: "Grove City DC access-control rollout", Status: "active", Lead: "maya",
		Description: "Readers, controllers, and door hardware for the new DC. Three trades, staged over four weeks.",
		StartDays:   -21, TargetDays: 14},
	{Key: "ls-summer-refresh", Customer: "lakeside", Location: "LS-CENTRAL-HS",
		Title: "Central High summer AP refresh", Status: "planned", Lead: "priya",
		Description: "Replace 40 aging access points across the high school before term starts.",
		StartDays:   30, TargetDays: 75},
	{Key: "ib-line3", Customer: "ironbridge", Location: "IB-PLANT1-L3",
		Title: "Line 3 controls modernization", Status: "completed", Lead: "maya",
		Description: "PLC and sensor replacement on line 3 during the scheduled shutdown.",
		StartDays:   -90, TargetDays: -40},
	{Key: "gw-rtu-firmware", Customer: "galewind", Location: "GW-SUB-A",
		Title: "Substation RTU firmware campaign", Status: "active", Lead: "gwen",
		Description: "Staged firmware upgrade across every RTU, one substation at a time, with rollback windows.",
		StartDays:   -35, TargetDays: 21},
	{Key: "bp-clinic-buildout", Customer: "brightpath", Location: "BP-WEST",
		Title: "West clinic IT buildout", Status: "active", Lead: "noor",
		Description: "Badge readers, label printers, and network for the new west clinic.",
		StartDays:   -14, TargetDays: 28},
	{Key: "cp-camera-upgrade", Customer: "cedarpoint", Location: "CP-YARD",
		Title: "Yard camera upgrade", Status: "planned", Lead: "iris",
		Description: "Swap the analog yard cameras for IP units and re-cable the pole runs.",
		StartDays:   21, TargetDays: 60},
	{Key: "su-hvac-controls", Customer: "summit", Location: "SU-TOWER",
		Title: "Tower HVAC controls retrofit", Status: "completed", Lead: "gwen",
		Description: "Replace the floor-level HVAC controllers and bring them onto the building network.",
		StartDays:   -120, TargetDays: -75},
	{Key: "hv-annex-access", Customer: "harborview", Location: "HV-WEST",
		Title: "Westside annex access control", Status: "canceled", Lead: "maya",
		Description: "Shelved when the annex lease was not renewed.",
		StartDays:   -60, TargetDays: -10},
}

// ------------------------------------------------------------------ hero tickets

type commentFixture struct {
	Author, Body  string
	Internal      bool
	RequestsReply bool
	// DayOffset places the comment relative to now; must fall after the
	// ticket's own created day or the thread reads out of order.
	DayOffset int
	// At is resolved from DayOffset during generation. Timestamps are fixed
	// before any writing so that writing consumes no randomness — see
	// seedTickets.
	At time.Time
}

type visitFixture struct {
	Status, Assignee, Notes string
	ScheduledDays           int
	CompletedDays           *int
	// Resolved during generation, like commentFixture.At.
	ScheduledAt time.Time
	CompletedAt *time.Time
}

type timeFixture struct {
	Staff, Note string
	Minutes     int
	WorkDays    int
	NonBillable bool
}

type ticketFixture struct {
	Key, Customer, Requester, Title, Body    string
	Priority, Source, Category, Type         string
	Location, LocationNote, Thing, ThingNote string
	Project, Assignee                        string
	EstimatedMinutes                         int
	OriginSubject                            string
	CreatedDays                              int
	// CreatedAt is resolved from CreatedDays during generation.
	CreatedAt   time.Time
	Transitions []string
	Comments    []commentFixture
	Visits      []visitFixture
	Time        []timeFixture
}

func ptr(i int) *int { return &i }

// Hand-written tickets: the ones worth pointing at in a demo. Everything else is
// generated. Keep the threads believable — this is the copy a viewer reads.
var heroTickets = []ticketFixture{
	{
		Key: "nw-reader-offline", Customer: "northwind", Requester: "regina",
		Title:    "North door reader is offline",
		Body:     "Staff badged in fine yesterday. This morning the reader has no lights and the door stays locked.",
		Priority: "high", Source: "portal", Category: "access-control", Type: "reactive",
		Location: "NW-HQ", Thing: "RDR-01", Assignee: "diego", EstimatedMinutes: 60,
		CreatedDays: -3, Transitions: []string{"in_progress"},
		Comments: []commentFixture{
			{Author: "diego", Body: "On my way — bringing a spare PoE injector in case it is the drop.", DayOffset: -3},
			{Author: "diego", Body: "Panel LED is dark. Suspect the switch port, not the reader.", Internal: true, DayOffset: -2},
			{Author: "regina", Body: "Thanks — side door is working so we are not blocked.", DayOffset: -2},
		},
		Visits: []visitFixture{{Status: "scheduled", ScheduledDays: 1, Assignee: "sam", Notes: "Bring PoE injector and a patch cable."}},
		Time:   []timeFixture{{Staff: "diego", Minutes: 25, WorkDays: -3, Note: "Remote triage, checked controller reachability."}},
	},
	{
		Key: "ib-pump-overcurrent", Customer: "ironbridge",
		Title:    "Pump fault on line 3",
		Body:     "Vibration sensor reporting overcurrent on the feed pump. Auto-filed by the plant controller.",
		Priority: "urgent", Source: "nats", Category: "iot-device", Type: "reactive",
		Location: "IB-PLANT1-L3", Thing: "PUMP-7", Assignee: "maya", EstimatedMinutes: 180,
		OriginSubject: "helpdesk.org_ironbridge.tickets.create",
		CreatedDays:   -5, Transitions: []string{"in_progress", "waiting"},
		Comments: []commentFixture{
			{Author: "maya", Body: "Pulled the trend — current spikes correlate with the 06:00 startup ramp.", Internal: true, DayOffset: -4},
			{Author: "maya", Body: "We have escalated to the pump vendor and are waiting on their engineer.", DayOffset: -3},
		},
		Time: []timeFixture{
			{Staff: "maya", Minutes: 95, WorkDays: -4, Note: "Trend analysis and vendor escalation."},
			{Staff: "tomas", Minutes: 40, WorkDays: -4, Note: "On-site inspection of the drive cabinet."},
		},
	},
	{
		Key: "hv-badge-intermittent", Customer: "harborview", Requester: "anita",
		Title:    "ER entrance badge reader intermittent",
		Body:     "Roughly one in five badges needs a second tap. Started after last week's power work.",
		Priority: "high", Source: "portal", Category: "access-control", Type: "reactive",
		Location: "HV-MAIN-ER", Thing: "HV-RDR-ER", Assignee: "priya",
		CreatedDays: -9, Transitions: []string{"in_progress", "resolved"},
		Comments: []commentFixture{
			{Author: "priya", Body: "Reseated the reader harness and reflashed the panel. Please keep an eye on it today.", RequestsReply: true, DayOffset: -7},
			{Author: "anita", Body: "No failed taps since this morning. Thank you.", DayOffset: -6},
		},
		Visits: []visitFixture{{Status: "completed", ScheduledDays: -7, Assignee: "sam", Notes: "Service entrance, ask for Anita.", CompletedDays: ptr(-7)}},
		Time:   []timeFixture{{Staff: "priya", Minutes: 75, WorkDays: -7, Note: "On-site diagnosis and harness reseat."}},
	},
	{
		Key: "nw-dc-install-doors", Customer: "northwind",
		Title:    "Install door hardware — DC dock doors 1-6",
		Body:     "Hang and wire controllers for the first six dock doors.",
		Priority: "normal", Source: "agent", Category: "access-control", Type: "planned",
		Location: "NW-DC1", Thing: "RDR-02", Project: "nw-dc-access", Assignee: "sam",
		EstimatedMinutes: 960, CreatedDays: -18, Transitions: []string{"in_progress"},
		Visits: []visitFixture{
			{Status: "completed", ScheduledDays: -14, Assignee: "sam", Notes: "Gate code 4417.", CompletedDays: ptr(-14)},
			{Status: "scheduled", ScheduledDays: 3, Assignee: "tomas", Notes: "Doors 4-6, bring the long ladder."},
		},
		Time: []timeFixture{
			{Staff: "sam", Minutes: 480, WorkDays: -14, Note: "Doors 1-3 hung and wired."},
			{Staff: "sam", Minutes: 120, WorkDays: -13, Note: "Rework on door 2 latch alignment.", NonBillable: true},
		},
	},
	{
		Key: "nw-dc-install-network", Customer: "northwind",
		Title:    "Install network drops — DC controller closet",
		Body:     "Twelve drops from the closet to the dock controllers.",
		Priority: "normal", Source: "agent", Category: "network", Type: "planned",
		Location: "NW-DC1", Project: "nw-dc-access", Assignee: "tomas", EstimatedMinutes: 600,
		CreatedDays: -16,
		Visits:      []visitFixture{{Status: "requested", Notes: "Needs scheduling once the doors are hung."}},
	},
	{
		Key: "cp-yard-camera", Customer: "cedarpoint", Requester: "owen",
		Title:    "Yard camera feed dropping overnight",
		Body:     "The northeast yard camera goes offline around 02:00 and comes back by 06:00.",
		Priority: "normal", Source: "webhook", Category: "network", Type: "reactive",
		Location: "CP-YARD", Thing: "CAM-NE", Assignee: "diego", CreatedDays: -6,
		Comments: []commentFixture{
			{Author: "diego", Body: "Could you confirm whether the pole lighting is on a timer? The window matches a power cycle.", RequestsReply: true, DayOffset: -5},
		},
	},
	{
		Key: "su-printer-jam", Customer: "summit", Requester: "lena",
		Title:    "Reception printer jamming constantly",
		Body:     "Jams every few pages on the lower tray.",
		Priority: "low", Source: "email", Category: "hardware", Type: "reactive",
		Location: "SU-TOWER", Thing: "Lobby Reception Printer", CreatedDays: -4,
		Comments: []commentFixture{{Author: "lena", Body: "Still happening this morning.", DayOffset: -2}},
	},
	{
		Key: "nw-unmatched-intake", Customer: "northwind",
		Title:    "Unknown device reporting faults",
		Body:     "Machine intake referenced a device code we do not have in the catalog yet.",
		Priority: "normal", Source: "nats", Category: "iot-device", Type: "reactive",
		OriginSubject: "helpdesk.org_northwind.tickets.create",
		// No Thing: the unmatched-code fallback the intakes actually produce. A
		// demo should show what that looks like in the UI.
		ThingNote: "NW-SENSOR-88", LocationNote: "somewhere on the mezzanine",
		CreatedDays: -2,
	},
	{
		Key: "gw-rtu-firmware-a1", Customer: "galewind", Requester: "bram",
		Title:    "Alpha RTU 1 firmware upgrade",
		Body:     "Stage the 3.2.0 firmware on Alpha RTU 1 during the Tuesday maintenance window.",
		Priority: "normal", Source: "agent", Category: "iot-device", Type: "planned",
		Location: "GW-SUB-A", Thing: "RTU-A1", Project: "gw-rtu-firmware", Assignee: "gwen",
		EstimatedMinutes: 240, CreatedDays: -22, Transitions: []string{"in_progress", "resolved", "closed"},
		Comments: []commentFixture{
			{Author: "gwen", Body: "Rollback image staged on the laptop before we touch anything.", Internal: true, DayOffset: -20},
			{Author: "gwen", Body: "Upgrade completed and verified against SCADA. No alarms through the window.", DayOffset: -19},
		},
		Visits: []visitFixture{{Status: "completed", ScheduledDays: -20, Assignee: "vic", Notes: "Two-person rule — coordinate with Bram at the gate.", CompletedDays: ptr(-20)}},
		Time: []timeFixture{
			{Staff: "gwen", Minutes: 180, WorkDays: -20, Note: "Firmware stage, upgrade, and SCADA verification."},
			{Staff: "vic", Minutes: 180, WorkDays: -20, Note: "Second person for the two-person rule."},
		},
	},
	{
		Key: "hv-annex-canceled", Customer: "harborview", Requester: "joel",
		Title:    "Annex reader install — on hold",
		Body:     "Hold the annex reader work until the lease question is settled.",
		Priority: "low", Source: "portal", Category: "access-control", Type: "planned",
		Location: "HV-WEST", Project: "hv-annex-access", CreatedDays: -55,
		Transitions: []string{"waiting", "closed"},
		Comments: []commentFixture{
			{Author: "maya", Body: "Closing this out — the annex lease was not renewed, so the project is canceled.", DayOffset: -12},
		},
	},
}

// ------------------------------------------------------------------ generator vocabulary

// Phrase tables for the generated bulk. Keyed loosely by thing type so a
// generated ticket about a projector doesn't read like one about a substation.
var issueTemplates = map[string][]string{
	"door-controller": {
		"Reader not accepting badges at %s",
		"Door held-open alarm firing on %s",
		"Controller lost network after power blip at %s",
		"Badge audit log missing entries — %s",
	},
	"kiosk": {
		"Kiosk stuck on splash screen at %s",
		"Touchscreen unresponsive in the corner — %s",
		"Kiosk printing blank check-in slips at %s",
	},
	"switch": {
		"Intermittent packet loss on the %s uplink",
		"Switch port flapping at %s",
		"Fiber uplink down at %s",
	},
	"plc":          {"Controller fault code on %s", "PLC dropped comms during shift change at %s", "Unexpected stop on %s"},
	"sensor":       {"Sensor readings drifting at %s", "Sensor offline since maintenance at %s", "Calibration overdue on %s"},
	"badge-reader": {"Badge reader needs repeated taps at %s", "Reader beeping continuously at %s", "Reader offline at %s"},
	"nurse-call":   {"Nurse call panel showing phantom alerts at %s", "Nurse call panel silent at %s"},
	"projector":    {"Projector very dim in %s", "Projector will not wake from standby — %s", "No signal to projector at %s"},
	"ap":           {"Wi-Fi dropping in %s", "Access point offline at %s", "Poor coverage reported at %s"},
	"camera":       {"Camera feed dropping at %s", "Camera view obstructed at %s", "Night vision not engaging on %s"},
	"gate":         {"Gate will not open on badge at %s", "Gate arm stuck halfway at %s"},
	"hvac":         {"HVAC controller unresponsive at %s", "Temperature setpoint not holding at %s"},
	"rtu":          {"RTU comms loss to SCADA at %s", "RTU reporting stale values at %s", "RTU watchdog reset overnight at %s"},
	"meter":        {"Revenue meter reading mismatch at %s", "Meter comms intermittent at %s"},
	"printer":      {"Label printer jamming at %s", "Printer offline at %s", "Labels printing misaligned at %s"},
	"":             {"Intermittent issue reported at %s", "Follow-up requested for %s", "Equipment check needed at %s"},
}

var installTemplates = []string{
	"Install replacement unit at %s",
	"Cable and terminate new drops at %s",
	"Commission new hardware at %s",
	"Site survey ahead of the refresh at %s",
	"Decommission and remove old equipment at %s",
}

var bodyOpeners = []string{
	"Reported this morning by the on-site contact.",
	"Noticed during the weekly walkthrough.",
	"Came in overnight; no one on site at the time.",
	"Third occurrence this month — logging it properly this time.",
	"Flagged by monitoring before anyone called it in.",
	"Called in by the site lead just after opening.",
}

var bodyDetails = []string{
	"No obvious pattern yet; happens at different times of day.",
	"Appears to clear itself after a power cycle, but keeps coming back.",
	"Started after last week's maintenance window.",
	"Only affects one area, everything else on that run is fine.",
	"Site staff have a workaround but want it fixed properly.",
	"Worth checking the cabling — that run was reworked recently.",
}

var staffReplies = []string{
	"Picked this up — starting with the obvious checks before anyone goes out.",
	"Reproduced remotely. Scheduling a visit rather than guessing at it.",
	"Swapped the suspect unit; monitoring before we call it done.",
	"Root cause looks like the upstream port, not the device itself.",
	"Parts are on order. I'll update as soon as they land.",
	"Cleared for now. Leaving the ticket open until it survives a full week.",
}

var internalNotes = []string{
	"Third call on this site in a month — worth a proper survey.",
	"Customer is sensitive about downtime here; schedule carefully.",
	"Old cabling on this run. Flagging for the account review.",
	"Vendor RMA is slow. Set expectations accordingly.",
}

var requesterReplies = []string{
	"Thanks — that's working again now.",
	"Still seeing it occasionally, but much better than before.",
	"Understood, we can work around it until then.",
	"Appreciate the quick turnaround on this one.",
	"Let me know if you need someone on site to let you in.",
}

var visitNotes = []string{
	"Check in at the front desk on arrival.",
	"Parking is tight — use the service lot.",
	"Bring a ladder, the unit is above the ceiling grid.",
	"Coordinate with the site contact before any outage.",
	"Gate code required; call ahead.",
}

var workNotes = []string{
	"On-site diagnosis and repair.",
	"Remote triage and log review.",
	"Cable testing and re-termination.",
	"Unit swap and verification.",
	"Follow-up check after the earlier fix.",
	"Rework after the initial attempt did not hold.",
}
