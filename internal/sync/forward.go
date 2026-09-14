package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/report"
)

func PlanForward(ctx context.Context, cfg config.Config, prov provider.Provider) (Plan, envfile.Document, error) {
	schema, err := envfile.ParseFile(cfg.SchemaFile, envfile.KindSchema)
	if err != nil {
		return Plan{}, envfile.Document{}, report.NewAppError("E004", report.ExitOperational, "schema file missing", "sync cannot build .env without the schema", "create .env.example or run 'ds init'", err)
	}
	local, err := envfile.ParseFile(cfg.EnvFile, envfile.KindLocal)
	if err != nil && !os.IsNotExist(err) {
		return Plan{}, envfile.Document{}, report.NewAppError("E006", report.ExitOperational, "local env file could not be read", "sync cannot compare the current .env file", "fix the file and retry", err)
	}
	if os.IsNotExist(err) {
		local = envfile.Document{Path: cfg.EnvFile, Kind: envfile.KindLocal, LineEnding: schema.LineEnding, TrailingNewline: schema.TrailingNewline}
	}
	plan, target, err := PlanForwardDocs(ctx, cfg, schema, local, prov)
	return plan, target, err
}

func PlanForwardDocs(ctx context.Context, cfg config.Config, schema, local envfile.Document, prov provider.Provider) (Plan, envfile.Document, error) {
	plan := Plan{Mode: "sync", Schema: schema, LocalEnv: local, Config: cfg}
	plan.Issues = append(plan.Issues, collectDocumentIssues(schema)...)
	plan.Issues = append(plan.Issues, collectDocumentIssues(local)...)
	sources, schemaIssues := classifySchema(cfg, schema)
	plan.Issues = append(plan.Issues, schemaIssues...)
	if len(plan.Issues) > 0 {
		return plan, envfile.Document{}, report.SilentExit(report.ExitValidation)
	}
	refs := map[string]string{}
	resolutions := map[string]provider.Resolution{}
	for _, line := range schema.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		switch sources[line.Key] {
		case config.SourceStatic:
			resolutions[line.Key] = provider.Resolution{Key: line.Key, Ref: line.Key, Value: line.Value, Source: string(config.SourceStatic)}
		case config.SourceProvider:
			refs[line.Key] = cfg.ProviderRef(line.Key)
		}
	}
	if len(refs) > 0 {
		status, err := prov.CheckReadiness(ctx)
		if err != nil {
			return plan, envfile.Document{}, err
		}
		plan.ProviderStatus = status
		if status.Code != "" {
			return plan, envfile.Document{}, report.NewAppError(status.Code, report.ExitOperational, status.Problem, status.Impact, status.Action, nil)
		}
		providerResults, err := prov.ResolveMany(ctx, refs)
		if err != nil {
			var appErr *report.AppError
			if errors.As(err, &appErr) {
				return plan, envfile.Document{}, err
			}
			return plan, envfile.Document{}, report.NewAppError("E003", report.ExitOperational, "provider resolution failed", "sync cannot resolve provider-managed schema keys", "check the configured provider and retry", err)
		}
		for key, res := range providerResults {
			resolutions[key] = res
			plan.Resolutions = append(plan.Resolutions, res)
		}
	}
	target := schema.Clone()
	target.Kind = envfile.KindLocal
	target.Path = cfg.EnvFile
	if local.LineEnding != "" {
		target.LineEnding = local.LineEnding
		target.TrailingNewline = local.TrailingNewline
	}
	localAssignments := local.AssignmentMap()
	for i, line := range target.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		source := sources[line.Key]
		before, ok := localAssignments[line.Key]
		localMissing := false
		if source == config.SourceLocal {
			localMissing = !ok || strings.TrimSpace(before.Value) == ""
			if localMissing {
				line.Value = ""
				plan.Warnings = append(plan.Warnings, localValueWarning(cfg, line.Key))
			} else {
				line.Value = before.Value
			}
			target.Lines[i] = line
			if localMissing {
				if !ok || (strings.TrimSpace(before.Value) == "" && before.Prefix == line.Prefix && before.Suffix == line.Suffix) {
					continue
				}
			}
		}
		res := resolutions[line.Key]
		marker := report.MarkerForSource(string(source))
		if source == config.SourceProvider {
			marker = report.MarkerForSource(res.Source)
		}
		if source != config.SourceLocal && (res.Source == "provider" || res.Source == "static") {
			line.Value = res.Value
			target.Lines[i] = line
		}
		switch {
		case source == config.SourceLocal:
			if localMissing {
				plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "update", File: "local", Before: report.RedactValue(before.Value), After: marker, Message: "blank local assignment will be normalized"})
			} else if before.Value != line.Value || before.Prefix != line.Prefix || before.Suffix != line.Suffix {
				plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "update", File: "local", Before: report.RedactValue(before.Value), After: marker, Message: "local value is preserved"})
			} else {
				plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "unchanged", File: "local", Before: report.RedactValue(before.Value), After: marker, Message: "local value is preserved"})
			}
		case res.Source == "missing" || res.Source == "error":
			plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "missing", File: "local", After: report.MarkerForSource("missing"), Message: "provider value unavailable"})
			plan.Issues = append(plan.Issues, ValidationIssue{Code: "E005", Severity: "error", File: cfg.EnvFile, Key: line.Key, Message: "secret not found for schema key", Action: "add the secret or mapping, then rerun"})
		case !ok:
			plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "add", File: "local", After: marker, Message: "will be added"})
		case before.Value != line.Value || before.Suffix != line.Suffix || before.Prefix != line.Prefix:
			plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "update", File: "local", Before: report.RedactValue(before.Value), After: marker, Message: "will be updated"})
		default:
			plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: "unchanged", File: "local", After: marker, Message: "already current"})
		}
	}
	localKeys := local.AssignmentMap()
	for key := range localKeys {
		if _, ok := schema.AssignmentMap()[key]; !ok {
			plan.Changes = append(plan.Changes, ChangeRecord{Key: key, ChangeType: "extra", File: "local", Message: "not present in schema"})
		}
	}
	if len(plan.Issues) > 0 {
		return plan, target, report.SilentExit(report.ExitValidation)
	}
	plan.WriteRequired = true
	if local.Raw != "" && string(envfile.Render(target)) == local.Raw {
		plan.WriteRequired = false
	}
	return plan, target, nil
}

func ApplyForward(target envfile.Document) error {
	_, err := envfile.WriteDocument(target.Path, target)
	if err != nil {
		return fmt.Errorf("write env file: %w", err)
	}
	return nil
}
