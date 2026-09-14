package sync

import (
	"context"
	"os"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/report"
)

func PlanReverse(_ context.Context, cfg config.Config) (Plan, envfile.Document, error) {
	schema, err := envfile.ParseFile(cfg.SchemaFile, envfile.KindSchema)
	if err != nil {
		return Plan{}, envfile.Document{}, report.NewAppError("E004", report.ExitOperational, "schema file missing", "reverse needs a schema file to update", "create .env.example or run 'ds init'", err)
	}
	local, err := envfile.ParseFile(cfg.EnvFile, envfile.KindLocal)
	if err != nil {
		return Plan{}, envfile.Document{}, report.NewAppError("E006", report.ExitOperational, "local env file missing", "reverse needs .env to discover new keys", "create .env or choose --env", err)
	}
	plan := Plan{Mode: "reverse", Schema: schema, LocalEnv: local, Config: cfg}
	plan.Issues = append(plan.Issues, collectDocumentIssues(schema)...)
	plan.Issues = append(plan.Issues, collectDocumentIssues(local)...)
	if len(plan.Issues) > 0 {
		return plan, envfile.Document{}, report.SilentExit(report.ExitValidation)
	}
	target, added := envfile.ReverseMerge(schema, local)
	_, schemaIssues := classifySchema(cfg, target)
	plan.Issues = append(plan.Issues, schemaIssues...)
	if len(schemaIssues) > 0 {
		return plan, target, issueAsValidationError(schemaIssues[0], "reverse cannot produce a valid .env.example")
	}
	for _, key := range added {
		marker := report.MarkerForSource("missing")
		if cfg.IsLocalKey(key) {
			marker = report.MarkerForSource(string(config.SourceLocal))
		}
		plan.Changes = append(plan.Changes, ChangeRecord{Key: key, ChangeType: "add", File: "schema", After: marker, Message: "blank placeholder will be added"})
	}
	plan.WriteRequired = len(added) > 0
	if !plan.WriteRequired {
		return plan, target, nil
	}
	return plan, target, nil
}

func PlanInit(cfg config.Config) (Plan, envfile.Document, error) {
	local, err := envfile.ParseFile(cfg.EnvFile, envfile.KindLocal)
	if err != nil {
		return Plan{}, envfile.Document{}, report.NewAppError("E006", report.ExitOperational, "local env file missing", "init needs .env as a source", "create .env or choose --env", err)
	}
	plan := Plan{Mode: "init", LocalEnv: local, Config: cfg}
	plan.Issues = append(plan.Issues, collectDocumentIssues(local)...)
	if len(plan.Issues) > 0 {
		return plan, envfile.Document{}, issueAsValidationError(plan.Issues[0], "init cannot generate .env.example from the current .env")
	}
	target := envfile.InitSchemaFromEnvWithLocalKeys(local, cfg.IsLocalKey)
	target.Path = cfg.SchemaFile
	_, schemaIssues := classifySchema(cfg, target)
	plan.Issues = append(plan.Issues, schemaIssues...)
	if len(schemaIssues) > 0 {
		return plan, target, issueAsValidationError(schemaIssues[0], "init cannot generate a valid .env.example")
	}
	for _, line := range target.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		changeType := "add"
		marker := report.MarkerForSource("static")
		if cfg.ClassifySource(line.Key, line.ManagedByProvider) == config.SourceLocal {
			marker = report.MarkerForSource(string(config.SourceLocal))
		} else if line.ManagedByProvider {
			marker = report.MarkerForSource("missing")
		}
		plan.Changes = append(plan.Changes, ChangeRecord{Key: line.Key, ChangeType: changeType, File: "schema", After: marker, Message: "schema entry prepared"})
	}
	plan.WriteRequired = true
	if existing, err := os.ReadFile(cfg.SchemaFile); err == nil && string(envfile.Render(target)) == string(existing) {
		plan.WriteRequired = false
	}
	return plan, target, nil
}
