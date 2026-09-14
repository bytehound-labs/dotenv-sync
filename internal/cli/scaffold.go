package cli

import (
	"errors"
	"fmt"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider/keepass"
	"dotenv-sync/internal/report"
	"github.com/spf13/cobra"
)

func newScaffoldCommand(s streams, opts *rootOptions) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Seed KeePass with blank entries for provider-managed schema keys",
		Long: `scaffold reads .env.example and creates a blank KeePass entry for every
provider-managed key (blank value in .env.example, excluding local_keys) that
does not already exist in the configured group. Existing entries are skipped,
never overwritten.

After scaffold, open KeePassXC, fill in the real secret values, then run ds sync.
This bootstraps the entry structure only — it does not push current .env values
back into KeePass.

Only supported when provider is keepass.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, setupPassword, err := ensureConfig(s, opts)
			if err != nil {
				return err
			}

			// scaffold is KeePass-only.
			if cfg.Provider != "keepass" {
				return report.NewAppError("E007", report.ExitOperational,
					"scaffold requires provider: keepass",
					"scaffold cannot create entries for providers other than KeePass",
					"set provider: keepass in .envsync.yaml and retry", nil)
			}

			// Parse the schema to find provider-managed keys (blank values).
			schema, err := envfile.ParseFile(cfg.SchemaFile, envfile.KindSchema)
			if err != nil {
				return report.NewAppError("E004", report.ExitOperational,
					"schema file missing",
					"scaffold cannot determine which keys to create",
					"create .env.example or run 'ds init'", err)
			}
			if issues := cfg.ValidateLocalKeys(schema); len(issues) > 0 {
				issue := issues[0]
				return report.NewAppError("E012", report.ExitValidation,
					issue.Message+": "+issue.Key,
					"scaffold cannot determine a valid provider/local schema split",
					issue.Action, nil)
			}

			// Collect keys that need entries — local keys never enter KeePass.
			var keys []string
			for _, line := range schema.Lines {
				if line.LineType == envfile.LineAssignment && cfg.ClassifySource(line.Key, line.ManagedByProvider) == config.SourceProvider {
					keys = append(keys, line.Key)
				}
			}

			if len(keys) == 0 {
				fmt.Fprintln(s.stdout, "No provider-managed keys found in schema — nothing to scaffold.")
				return nil
			}

			if dryRun {
				fmt.Fprintf(s.stdout, "Would scaffold %d key(s) into %s (group: %s):\n", len(keys), cfg.KeePassDatabase, cfg.KeePassGroup)
				for _, key := range keys {
					fmt.Fprintln(s.stdout, report.ChangeLine("add", key, "[DRY-RUN]"))
				}
				return nil
			}

			// Build the adapter. If setup just ran, reuse the password already
			// collected so the user is not prompted a second time.
			adapter := keepass.NewAdapter(cfg)
			if setupPassword != "" {
				adapter.SetPassword(setupPassword)
			} else if err := adapter.EnsurePassword(cmd.Context()); err != nil {
				return report.NewAppError("E003", report.ExitOperational,
					"KeePass master password could not be read",
					"scaffold cannot create entries without vault access",
					"check your master password and retry", err)
			}

			created, skipped, failed := 0, 0, 0
			for _, key := range keys {
				err := adapter.CreateEntry(cmd.Context(), key)
				if err == nil {
					fmt.Fprintln(s.stdout, report.ChangeLine("add", key, "[CREATED]"))
					created++
				} else if errors.Is(err, keepass.ErrEntryExists) {
					fmt.Fprintln(s.stdout, report.ChangeLine("skip", key, "[EXISTS]"))
					skipped++
				} else {
					fmt.Fprintln(s.stdout, report.ChangeLine("error", key, "[FAILED]"))
					failed++
				}
			}

			summary := report.Summary{Added: created, Unchanged: skipped, Error: failed}
			fmt.Fprintln(s.stdout, report.SummaryLine(report.StatusWritten, cfg.KeePassDatabase, summary, ""))
			fmt.Fprintln(s.stdout, "Open KeePassXC, fill in the secret values, then run 'ds sync'.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without creating entries")
	return cmd
}
