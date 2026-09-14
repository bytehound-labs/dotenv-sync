package sync

import (
	"path/filepath"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/report"
)

type ChangeRecord struct {
	Key        string
	ChangeType string
	Before     string
	After      string
	File       string
	Message    string
}

type ValidationIssue struct {
	Code     string
	Severity string
	File     string
	Key      string
	Message  string
	Action   string
}

type Plan struct {
	Mode           string
	Schema         envfile.Document
	LocalEnv       envfile.Document
	Config         config.Config
	ProviderStatus provider.Status
	Resolutions    []provider.Resolution
	Changes        []ChangeRecord
	WriteRequired  bool
	Issues         []ValidationIssue
	Warnings       []ValidationIssue
}

func Summarize(changes []ChangeRecord) report.Summary {
	var summary report.Summary
	for _, change := range changes {
		switch change.ChangeType {
		case "add":
			summary.Added++
		case "update":
			summary.Updated++
		case "unchanged":
			summary.Unchanged++
		case "missing":
			summary.Missing++
		case "extra":
			summary.Extra++
		case "error":
			summary.Error++
		}
	}
	return summary
}

func SummarizePlan(plan Plan) report.Summary {
	summary := Summarize(plan.Changes)
	summary.Warnings = len(plan.Warnings)
	return summary
}

func collectDocumentIssues(doc envfile.Document) []ValidationIssue {
	issues := make([]ValidationIssue, 0, len(doc.ParseErrors)+len(doc.Duplicates))
	for _, parseErr := range doc.ParseErrors {
		issues = append(issues, ValidationIssue{Code: "E006", Severity: "error", File: filepath.Base(doc.Path), Message: parseErr, Action: "fix the file formatting and rerun validate"})
	}
	for _, duplicate := range doc.Duplicates {
		issues = append(issues, ValidationIssue{Code: "E008", Severity: "error", File: filepath.Base(doc.Path), Key: duplicate, Message: "duplicate key detected", Action: "remove the duplicate and rerun validate"})
	}
	return issues
}
