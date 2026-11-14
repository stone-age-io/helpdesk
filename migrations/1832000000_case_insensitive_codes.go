package migrations

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Code uniqueness stops folding case differently from the platform's.
//
// ADR 0003 rule 3 in platform-docs settles this for the ecosystem: a code is
// STORED as entered, compared for UNIQUENESS ignoring case, and looked up by a
// human ignoring case — while subjects and permissions stay exact, because NATS
// is case-sensitive and the stored spelling is the only spelling. The platform
// rebuilt its four code indexes `COLLATE NOCASE` and taught `stone` and the
// console's scanner to match with `:lower`. The helpdesk mirrors those same
// collections and joins upstream on the same codes, and did neither.
//
// Why uniqueness has to move together with the lookups (which this change also
// folds, in internal/ingest, internal/inbound and ScanView.vue): a
// case-insensitive lookup over a case-sensitive index is WORSE than what it
// replaces. One customer could legitimately hold both `RDR-01` and `rdr-01`,
// and `FindFirstRecordByFilter` would then return whichever row SQLite reached
// first — turning today's silent miss (the code drops into `thing_note`) into a
// silent wrong-record attach. Folding both makes the second row impossible, so
// a fold-case lookup can only ever match one row.
//
// ADR 0003's own line applies to this repo too: there is no data to sweep. If a
// deployment somehow already holds a case-only duplicate pair, `app.Save` fails
// here with the index violation rather than silently dropping a row, and an
// operator renames one of them first.
//
// Note this index is for UNIQUENESS, not for the lookups. SQLite applies a
// column's collation to a comparison, not an index's, so `code = 'door-1'`
// still would not match `DOOR-1`; that is why the resolvers use PocketBase's
// `:lower` modifier (→ `LOWER(code) = ?`) and pass an already-lowered param.
// `LOWER(code)` is an expression, so it does not use this index — a table scan
// over one customer's catalog, which is tens to low thousands of rows.
func init() {
	m.Register(caseInsensitiveCodesUp, caseInsensitiveCodesDown)
}

// codeIndexes names each (collection, index, columns) whose code half folds.
// `customers.code` is in the list and matters most: it is subject token 2, so
// an operator who types `ACME` where the platform minted `acme` sends every
// machine-filed ticket for that tenant down the unmapped-org path, which warns
// and ACKS. The platform mints organization codes lowercase (ADR 0002 rule 1),
// so the fold is purely defensive there — but a dropped ticket is unrecoverable
// and the defence costs one word.
var codeIndexes = []struct {
	collection string
	index      string
	columns    string // without the trailing COLLATE
}{
	{"locations", "idx_locations_code", "customer, code"},
	{"things", "idx_things_code", "customer, code"},
	{"thing_types", "idx_thing_types_code", "customer, code"},
	{"location_types", "idx_location_types_code", "customer, code"},
	{"customers", "idx_customers_code", "code"},
}

func caseInsensitiveCodesUp(app core.App) error {
	return rebuildCodeIndexes(app, true)
}

func caseInsensitiveCodesDown(app core.App) error {
	return rebuildCodeIndexes(app, false)
}

// rebuildCodeIndexes re-adds each code index with or without the NOCASE
// collation. AddIndex removes an index of the same name first, so re-adding is
// the replace, and the partial `code != ''` predicate is carried through
// unchanged: blanks are the normal state for a customer the platform never
// onboarded, and SQLite treats '' as a value rather than NULL.
func rebuildCodeIndexes(app core.App, nocase bool) error {
	for _, ci := range codeIndexes {
		col, err := app.FindCollectionByNameOrId(ci.collection)
		if err != nil {
			return fmt.Errorf("find %s: %w", ci.collection, err)
		}
		if strings.Contains(strings.ToUpper(col.GetIndex(ci.index)), "NOCASE") == nocase {
			continue // idempotent
		}
		columns := ci.columns
		if nocase {
			columns += " COLLATE NOCASE"
		}
		col.AddIndex(ci.index, true, columns, "code != ''")
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", ci.collection, err)
		}
	}
	return nil
}
