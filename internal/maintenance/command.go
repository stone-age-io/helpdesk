package maintenance

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// RegisterCommand wires the `maintenance-run` subcommand, following the shape
// of demoseed's `seed-demo`.
//
// Not merely a debug affordance: PocketBase's cron is process-local, so an
// install that was down at 03:45 does not generate that night and — for a
// schedule-anchored plan — silently loses the occurrence when the next live
// tick advances past it. This is the catch-up. It is safe to run any number of
// times: generation is idempotent per (plan, occurrence) through the dedupe key.
//
// No --dry-run flag on purpose. It would need its own copy of the query and the
// due test, which is exactly the kind of second implementation that drifts —
// and the question it answers ("what is due?") is already on the Maintenance
// roster, which shows next_due for every plan. Running this is the dry run.
//
//	./helpdesk maintenance-run
func RegisterCommand(app *pocketbase.PocketBase) {
	cmd := &cobra.Command{
		Use:   "maintenance-run",
		Short: "Generate tickets for maintenance plans that have come due",
		Long: "Runs the same sweep as the daily maintenance_generate cron, once, now.\n\n" +
			"Safe to re-run: each plan occurrence is generated at most once, guarded by\n" +
			"the ticket dedupe key. Use it to catch up after downtime, or to see a plan\n" +
			"work without waiting for 03:45.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Automigrate only hooks OnServe, and this is a one-shot subcommand.
			if err := app.Bootstrap(); err != nil {
				return fmt.Errorf("bootstrap: %w", err)
			}
			runner := core.NewMigrationsRunner(app, core.AppMigrations)
			if _, err := runner.Up(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}

			created, skipped, err := Generate(app, time.Now().UTC())
			if err != nil {
				return err
			}
			fmt.Printf("maintenance: created %d ticket(s), skipped %d plan(s) with work still open\n",
				created, skipped)
			return nil
		},
	}

	app.RootCmd.AddCommand(cmd)
}
