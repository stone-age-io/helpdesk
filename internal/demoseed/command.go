package demoseed

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// RegisterCommand wires the `seed-demo` subcommand onto the root command,
// following the kiosk sibling's `seed-catalog` shape.
//
// Guarded behind --confirm on purpose: this ships in the same binary an operator
// runs in production, and `helpdesk seed-demo` typo'd against a live install
// would inject eight fictional customers into a real desk. The flag is the whole
// safety mechanism — keep it required.
//
//	./helpdesk seed-demo --confirm
//	./helpdesk seed-demo --confirm --tickets 400
func RegisterCommand(app *pocketbase.PocketBase) {
	cmd := &cobra.Command{
		Use:   "seed-demo",
		Short: "Populate this instance with demo/showcase data (idempotent)",
		Long: "Seeds customers, locations, things, type taxonomies, staff, requesters, " +
			"projects, and a backdated ticket history with comments, visits, and logged " +
			"time.\n\nIdempotent: re-running tops up rather than duplicating, and a run " +
			"that fails partway heals on the next one.\n\nNOT for production installs — " +
			"every record is fictional and every login shares a well-known password.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			confirm, _ := cmd.Flags().GetBool("confirm")
			tickets, _ := cmd.Flags().GetInt("tickets")
			seed, _ := cmd.Flags().GetInt64("seed")

			if !confirm {
				return fmt.Errorf(
					"refusing to seed without --confirm\n\n"+
						"This writes fictional customers, staff, and tickets into this instance,\n"+
						"and every seeded login shares the password %q.\n"+
						"Never run it against a production desk.", DemoPassword)
			}

			// Automigrate only hooks OnServe, and this is a one-shot subcommand.
			if err := app.Bootstrap(); err != nil {
				return fmt.Errorf("bootstrap: %w", err)
			}
			runner := core.NewMigrationsRunner(app, core.AppMigrations)
			if _, err := runner.Up(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}

			res, err := Run(app, Options{
				Tickets: tickets,
				Seed:    seed,
				Log:     func(format string, args ...any) { fmt.Printf(format+"\n", args...) },
			})
			if res != nil {
				fmt.Print("\n", res.Summary())
			}
			if err != nil {
				return err
			}
			fmt.Printf("\ndone — every demo login uses password %q\n", DemoPassword)
			return nil
		},
	}

	cmd.Flags().Bool("confirm", false, "required: acknowledge this writes fictional data into this instance")
	cmd.Flags().Int("tickets", 150, "total tickets to converge on, curated fixtures included")
	cmd.Flags().Int64("seed", 20260827, "PRNG seed; changing it regenerates different (but still stable) bulk")

	app.RootCmd.AddCommand(cmd)
}
