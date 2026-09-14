package config

import (
	"strings"

	"dotenv-sync/internal/envfile"
)

type ValueSource string

const (
	SourceStatic   ValueSource = "static"
	SourceProvider ValueSource = "provider"
	SourceLocal    ValueSource = "local"
)

type SchemaIssue struct {
	Key     string
	Message string
	Action  string
}

func (c Config) ClassifySource(key string, providerManaged bool) ValueSource {
	if c.IsLocalKey(key) {
		return SourceLocal
	}
	if providerManaged {
		return SourceProvider
	}
	return SourceStatic
}

func (c Config) ValidateLocalKeys(schema envfile.Document) []SchemaIssue {
	counts := map[string]int{}
	lines := map[string]envfile.EnvironmentLine{}
	for _, line := range schema.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		counts[line.Key]++
		lines[line.Key] = line
	}

	issues := make([]SchemaIssue, 0)
	for _, key := range c.LocalKeys {
		switch counts[key] {
		case 0:
			issues = append(issues, SchemaIssue{
				Key:     key,
				Message: "local key is not declared in the schema",
				Action:  "add a blank assignment to .env.example or remove it from local_keys",
			})
			continue
		case 1:
			// Continue with the blank-value check below.
		default:
			issues = append(issues, SchemaIssue{
				Key:     key,
				Message: "local key must appear exactly once in the schema",
				Action:  "keep one blank assignment in .env.example",
			})
			continue
		}

		if strings.TrimSpace(lines[key].Value) != "" {
			issues = append(issues, SchemaIssue{
				Key:     key,
				Message: "local key must be blank in the schema",
				Action:  "change the schema assignment to a blank value",
			})
		}
		if _, mapped := c.Mapping[key]; mapped {
			issues = append(issues, SchemaIssue{
				Key:     key,
				Message: "local key must not be provider-mapped",
				Action:  "remove the key from mapping",
			})
		}
	}
	return issues
}
