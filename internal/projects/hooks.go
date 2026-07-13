// Package projects owns project lifecycle glue that can't live in collection
// rules: sequential number assignment and field defaults. It mirrors
// internal/tickets — a project is the durable container grouping 1..N tickets
// for a customer's installation / field work.
package projects

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds the project create hook: a sequential `number` (the unique
// index is the collision backstop) and a `planned` status default. PocketBase
// serializes writes on one SQLite connection.
func Register(app *pocketbase.PocketBase) {
	app.OnRecordCreate("projects").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetInt("number") == 0 {
			e.Record.Set("number", nextNumber(e.App))
		}
		if e.Record.GetString("status") == "" {
			e.Record.Set("status", "planned")
		}
		return e.Next()
	})
}

func nextNumber(app core.App) int {
	var max int
	_ = app.DB().
		Select("COALESCE(MAX(number), 0)").
		From("projects").
		Row(&max)
	return max + 1
}
