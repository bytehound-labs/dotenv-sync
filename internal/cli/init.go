package cli

import (
	"fmt"

	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/report"
	syncpkg "dotenv-sync/internal/sync"
	"github.com/spf13/cobra"
)

func newInitCommand(s streams, opts *rootOptions) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate .env.example from .env, with first-run provider setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			setupRan, cfg, _, err := ensureConfig(s, opts)
			if err != nil {
				return err
			}
			if setupRan {
				fmt.Fprintln(s.stdout, "Run 'ds sync' to populate your .env file.")
				return nil
			}

			plan, target, err := syncpkg.PlanInit(cfg)
			for _, change := range plan.Changes {
				fmt.Fprintln(s.stdout, report.ChangeLine(change.ChangeType, change.Key, change.After))
			}
			if err != nil {
				return err
			}
			summary := syncpkg.Summarize(plan.Changes)
			if dryRun {
				fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusChecked, cfg.SchemaFile, summary, "dry-run"))
				return nil
			}
			if !plan.WriteRequired {
				fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusUnchanged, cfg.SchemaFile, summary, "already up to date"))
				return nil
			}
			if _, err := envfile.WriteDocument(cfg.SchemaFile, target); err != nil {
				return report.NewAppError("E006", report.ExitOperational, "schema file could not be written", "init could not update .env.example", "check file permissions and retry", err)
			}
			fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusWritten, cfg.SchemaFile, summary, ""))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	return cmd
}
