package envfile

import "strings"

func InitSchemaFromEnv(local Document) Document {
	return InitSchemaFromEnvWithLocalKeys(local, nil)
}

func InitSchemaFromEnvWithLocalKeys(local Document, isLocalKey func(string) bool) Document {
	schema := local.Clone()
	schema.Kind = KindSchema
	for i, line := range schema.Lines {
		if line.LineType != LineAssignment {
			continue
		}
		if (isLocalKey != nil && isLocalKey(line.Key)) || IsSecretLike(line.Key, line.Value) {
			line.Value = ""
			line.ManagedByProvider = true
		}
		schema.Lines[i] = line
	}
	return schema
}

func ReverseMerge(schema, local Document) (Document, []string) {
	result := schema.Clone()
	result.Kind = KindSchema
	schemaKeys := schema.AssignmentMap()
	added := make([]string, 0)
	for _, line := range local.Lines {
		if line.LineType != LineAssignment {
			continue
		}
		if _, ok := schemaKeys[line.Key]; ok {
			continue
		}
		added = append(added, line.Key)
		result.Lines = append(result.Lines, EnvironmentLine{
			Index:             len(result.Lines),
			LineType:          LineAssignment,
			Key:               line.Key,
			Value:             "",
			ManagedByProvider: true,
			Prefix:            line.Key + "=",
		})
		schemaKeys[line.Key] = line
	}
	return result, added
}

func IsSecretLike(key, value string) bool {
	upper := strings.ToUpper(key)
	for _, token := range []string{"SECRET", "TOKEN", "PASSWORD", "PASS", "KEY", "DATABASE_URL", "URL", "DSN", "CREDENTIAL"} {
		if strings.Contains(upper, token) {
			return true
		}
	}
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "sk_") || strings.HasPrefix(trimmed, "ghp_") || strings.Contains(trimmed, "://")
}
