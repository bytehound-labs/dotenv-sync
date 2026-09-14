package sync

import (
	"context"
	"errors"
	"fmt"
	"os"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/report"
)

const pushWritableFieldPassword = "password"

func PlanPush(ctx context.Context, cfg config.Config, prov provider.PushProvider) (Plan, provider.EnvPayload, error) {
	schema, err := envfile.ParseFile(cfg.SchemaFile, envfile.KindSchema)
	if err != nil {
		return Plan{}, provider.EnvPayload{}, report.NewAppError("E004", report.ExitOperational, "schema file missing", "push needs .env.example as schema context", "create .env.example or run 'ds init'", err)
	}
	local, err := envfile.ParseFile(cfg.EnvFile, envfile.KindLocal)
	if err != nil {
		if os.IsNotExist(err) {
			return Plan{}, provider.EnvPayload{}, report.NewAppError("E006", report.ExitOperational, "local env file missing", "push needs .env as the upload source", "create .env or choose --env", err)
		}
		return Plan{}, provider.EnvPayload{}, report.NewAppError("E006", report.ExitOperational, "local env file could not be read", "push cannot load the current .env", "fix the file and retry", err)
	}
	return PlanPushDocs(ctx, cfg, schema, local, prov)
}

func PlanPushDocs(ctx context.Context, cfg config.Config, schema, local envfile.Document, prov provider.PushProvider) (Plan, provider.EnvPayload, error) {
	plan := Plan{Mode: "push", Schema: schema, LocalEnv: local, Config: cfg}
	plan.Issues = append(plan.Issues, collectDocumentIssues(schema)...)
	plan.Issues = append(plan.Issues, collectDocumentIssues(local)...)
	if len(plan.Issues) > 0 {
		return plan, provider.EnvPayload{}, issueAsAppError(plan.Issues[0], "push cannot upload the current .env")
	}
	_, schemaIssues := classifySchema(cfg, schema)
	plan.Issues = append(plan.Issues, schemaIssues...)
	if len(schemaIssues) > 0 {
		return plan, provider.EnvPayload{}, issueAsValidationError(schemaIssues[0], "push cannot upload the current .env")
	}
	localEnv := CanonicalDocumentEnv(local)
	fieldRefs := map[string]string{}
	fieldTargetEnv := map[string]string{}
	if !cfg.UsesNoteJSON() {
		var err error
		fieldRefs, fieldTargetEnv, err = buildFieldsPushTarget(cfg, schema, local)
		if err != nil {
			return plan, provider.EnvPayload{}, err
		}
	}
	status, err := prov.CheckReadiness(ctx)
	if err != nil {
		return plan, provider.EnvPayload{}, err
	}
	plan.ProviderStatus = status
	if status.Code != "" {
		return plan, provider.EnvPayload{}, report.NewAppError(status.Code, report.ExitOperational, status.Problem, status.Impact, status.Action, nil)
	}
	current, err := prov.LoadEnvPayload(ctx)
	if err != nil {
		return plan, provider.EnvPayload{}, err
	}
	if !cfg.UsesNoteJSON() {
		currentEnv, err := loadFieldsPushCurrentEnv(ctx, prov, fieldRefs)
		if err != nil {
			return plan, provider.EnvPayload{}, err
		}
		current.Env = currentEnv
		target := provider.EnvPayload{
			ItemName:    current.ItemName,
			StorageMode: cfg.StorageMode,
			Exists:      current.Exists,
			Notes:       current.Notes,
			Password:    current.Password,
			Env:         fieldTargetEnv,
		}
		if target.ItemName == "" {
			target.ItemName = cfg.ItemName
		}
		plan.Changes = buildPushChanges(schema.AssignmentMap(), localEnv, target.Env, current.Env)
		plan.WriteRequired = !NoteJSONEqual(target.Env, current.Env)
		return plan, target, nil
	}
	target := provider.EnvPayload{
		ItemName:    cfg.ItemName,
		StorageMode: cfg.StorageMode,
		Exists:      current.Exists,
		Format:      NoteJSONFormat,
		Password:    current.Password,
		Env:         CanonicalDocumentEnvExcept(local, cfg.IsLocalKey),
	}
	plan.Changes = buildPushChanges(schema.AssignmentMap(), localEnv, target.Env, current.Env)
	plan.WriteRequired = !NoteJSONEqual(target.Env, current.Env) || !current.Exists
	if !current.Exists && len(target.Env) == 0 {
		plan.WriteRequired = false
	}
	return plan, target, nil
}

func buildPushChanges(schemaKeys map[string]envfile.EnvironmentLine, localEnv, targetEnv, providerEnv map[string]string) []ChangeRecord {
	keys := unionKeys(targetEnv, providerEnv)
	changes := make([]ChangeRecord, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		seen[key] = struct{}{}
		localValue, hasLocal := targetEnv[key]
		providerValue, hasProvider := providerEnv[key]
		if _, inSchema := schemaKeys[key]; !inSchema {
			localValue, hasLocal := localEnv[key]
			switch {
			case hasLocal:
				changes = append(changes, ChangeRecord{
					Key:        key,
					ChangeType: "extra",
					Before:     report.RedactValue(providerValue),
					After:      report.RedactValue(localValue),
					File:       "provider",
					Message:    "present in .env but not in .env.example",
				})
			case hasProvider:
				changes = append(changes, ChangeRecord{Key: key, ChangeType: "update", File: "provider", Before: report.RedactValue(providerValue), After: report.RedactValue(""), Message: "will be removed from Bitwarden"})
			}
			continue
		}
		switch {
		case hasLocal && !hasProvider:
			changes = append(changes, ChangeRecord{Key: key, ChangeType: "add", File: "provider", After: report.RedactValue(localValue), Message: "will be added to Bitwarden"})
		case !hasLocal && hasProvider:
			changes = append(changes, ChangeRecord{Key: key, ChangeType: "update", File: "provider", Before: report.RedactValue(providerValue), After: report.RedactValue(""), Message: "will be removed from Bitwarden"})
		case localValue != providerValue:
			changes = append(changes, ChangeRecord{Key: key, ChangeType: "update", File: "provider", Before: report.RedactValue(providerValue), After: report.RedactValue(localValue), Message: "will be updated in Bitwarden"})
		default:
			changes = append(changes, ChangeRecord{Key: key, ChangeType: "unchanged", File: "provider", After: report.RedactValue(localValue), Message: "already current"})
		}
	}
	for _, key := range report.SortedKeys(localEnv) {
		if _, inSchema := schemaKeys[key]; inSchema {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		changes = append(changes, ChangeRecord{
			Key:        key,
			ChangeType: "extra",
			After:      report.RedactValue(localEnv[key]),
			File:       "provider",
			Message:    "present in .env but not in .env.example",
		})
	}
	return changes
}

func unionKeys(left, right map[string]string) []string {
	set := map[string]struct{}{}
	for key := range left {
		set[key] = struct{}{}
	}
	for key := range right {
		set[key] = struct{}{}
	}
	return report.SortedKeys(set)
}

func issueAsAppError(issue ValidationIssue, impact string) error {
	return issueAsAppErrorWithExit(issue, report.ExitOperational, impact)
}

func issueAsValidationError(issue ValidationIssue, impact string) error {
	return issueAsAppErrorWithExit(issue, report.ExitValidation, impact)
}

func issueAsAppErrorWithExit(issue ValidationIssue, exitCode int, impact string) error {
	problem := issue.Message
	if issue.Key != "" {
		problem = fmt.Sprintf("%s: %s", issue.Message, issue.Key)
	}
	return report.NewAppError(issue.Code, exitCode, problem, impact, issue.Action, nil)
}

func PreserveAppError(err error, fallback func(error) error) error {
	if err == nil {
		return nil
	}
	var appErr *report.AppError
	if errors.As(err, &appErr) {
		return err
	}
	return fallback(err)
}

func buildFieldsPushTarget(cfg config.Config, schema, local envfile.Document) (map[string]string, map[string]string, error) {
	localAssignments := local.AssignmentMap()
	refs := map[string]string{}
	targetEnv := map[string]string{}
	fieldValues := map[string]string{}
	for _, line := range schema.Lines {
		if line.LineType != envfile.LineAssignment || cfg.ClassifySource(line.Key, line.ManagedByProvider) != config.SourceProvider {
			continue
		}
		localLine, ok := localAssignments[line.Key]
		if !ok || localLine.Value == "" {
			continue
		}
		ref := cfg.ProviderRef(line.Key)
		if ref != pushWritableFieldPassword {
			return nil, nil, report.NewAppError("E011", report.ExitOperational, "fields-mode push only supports mappings to the Bitwarden password field", "push cannot safely update custom Bitwarden fields through rbw", "map pushed keys to password, or switch to storage_mode: note_json and retry", nil)
		}
		if current, ok := fieldValues[ref]; ok && current != localLine.Value {
			return nil, nil, report.NewAppError("E011", report.ExitOperational, "multiple env keys map to the Bitwarden password field with different values", "push cannot safely collapse conflicting local values into one Bitwarden field", "use one value per shared password field, or switch to storage_mode: note_json and retry", nil)
		}
		fieldValues[ref] = localLine.Value
		refs[line.Key] = ref
		targetEnv[line.Key] = localLine.Value
	}
	return refs, targetEnv, nil
}

func loadFieldsPushCurrentEnv(ctx context.Context, prov provider.Provider, refs map[string]string) (map[string]string, error) {
	if len(refs) == 0 {
		return map[string]string{}, nil
	}
	results, err := prov.ResolveMany(ctx, refs)
	if err != nil {
		return nil, PreserveAppError(err, func(err error) error {
			return report.NewAppError("E003", report.ExitOperational, "provider field could not be loaded", "push cannot read the repo-scoped Bitwarden item", "check rbw and retry", err)
		})
	}
	currentEnv := map[string]string{}
	for key, res := range results {
		switch res.Source {
		case "provider":
			currentEnv[key] = res.Value
		case "missing":
			continue
		default:
			problem := "provider field could not be loaded"
			if key != "" {
				problem = fmt.Sprintf("provider field could not be loaded: %s", key)
			}
			return nil, report.NewAppError("E003", report.ExitOperational, problem, "push cannot read the repo-scoped Bitwarden item", "check rbw and retry", nil)
		}
	}
	return currentEnv, nil
}
