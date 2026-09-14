package cli

import (
	"errors"
	"fmt"

	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/report"
	syncpkg "dotenv-sync/internal/sync"
	"github.com/spf13/cobra"
)

func newSyncCommand(s streams, opts *rootOptions) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Create or refresh .env from the schema and provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			plan, target, err := syncpkg.PlanForward(cmd.Context(), cfg, providerFor(cfg))
			var appErr *report.AppError
			if err != nil && errors.As(err, &appErr) {
				return err
			}
			if dryRun {
				for _, change := range plan.Changes {
					fmt.Fprintln(s.stdout, report.ChangeLine(change.ChangeType, change.Key, change.After))
				}
			} else {
				for _, change := range plan.Changes {
					if change.ChangeType == "missing" {
						fmt.Fprintln(s.stdout, report.ChangeLine(change.ChangeType, change.Key, change.After))
					}
				}
			}
			printWarnings(s.stdout, plan.Warnings)
			if err != nil {
				return err
			}
			summary := syncpkg.SummarizePlan(plan)
			if dryRun {
				if plan.WriteRequired {
					fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusChecked, cfg.EnvFile, summary, "dry-run"))
				} else {
					fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusUnchanged, cfg.EnvFile, summary, "already up to date"))
				}
				_ = target
				return nil
			}
			if !plan.WriteRequired {
				fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusUnchanged, cfg.EnvFile, summary, "already up to date"))
				return nil
			}
			if _, err := envfile.WriteDocument(cfg.EnvFile, target); err != nil {
				return report.NewAppError("E006", report.ExitOperational, "env file could not be written", "sync could not update .env", "check file permissions and retry", err)
			}
			for _, change := range plan.Changes {
				if change.ChangeType == "add" || change.ChangeType == "update" || change.ChangeType == "extra" {
					fmt.Fprintln(s.stdout, report.ChangeLine(change.ChangeType, change.Key, change.After))
				}
			}
			fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusWritten, cfg.EnvFile, summary, ""))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	return cmd
}
