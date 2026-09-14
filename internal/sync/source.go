package sync

import (
	"path/filepath"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
)

const localValueWarningCode = "W001"

func classifySchema(cfg config.Config, schema envfile.Document) (map[string]config.ValueSource, []ValidationIssue) {
	sources := make(map[string]config.ValueSource)
	for _, line := range schema.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		sources[line.Key] = cfg.ClassifySource(line.Key, line.ManagedByProvider)
	}

	issues := make([]ValidationIssue, 0)
	for _, issue := range cfg.ValidateLocalKeys(schema) {
		issues = append(issues, ValidationIssue{
			Code:     "E012",
			Severity: "error",
			File:     filepath.Base(schema.Path),
			Key:      issue.Key,
			Message:  issue.Message,
			Action:   issue.Action,
		})
	}
	return sources, issues
}

func localValueWarning(cfg config.Config, key string) ValidationIssue {
	return ValidationIssue{
		Code:     localValueWarningCode,
		Severity: "warning",
		File:     filepath.Base(cfg.EnvFile),
		Key:      key,
		Message:  "local value is missing or blank; set it in .env (it is never stored in a provider)",
		Action:   "set the value in .env",
	}
}
