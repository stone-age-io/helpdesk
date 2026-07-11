// Package tickets owns ticket lifecycle glue that can't live in collection
// rules: sequential number assignment and field defaults.
package tickets

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds the ticket create hook: assigns the next sequential ticket
// number and defaults status/priority/source when the writer omitted them.
// PocketBase serializes writes on one SQLite connection, and the unique index
// on `number` is the collision backstop.
func Register(app *pocketbase.PocketBase) {
	app.OnRecordCreate("tickets").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetInt("number") == 0 {
			e.Record.Set("number", nextNumber(e.App))
		}
		if e.Record.GetString("status") == "" {
			e.Record.Set("status", "open")
		}
		if e.Record.GetString("priority") == "" {
			e.Record.Set("priority", "normal")
		}
		if e.Record.GetString("source") == "" {
			e.Record.Set("source", "agent")
		}
		return e.Next()
	})
}

func nextNumber(app core.App) int {
	var max int
	_ = app.DB().
		Select("COALESCE(MAX(number), 0)").
		From("tickets").
		Row(&max)
	return max + 1
}
