package report

import "strings"

func RedactValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[EMPTY]"
	}
	return "[REDACTED]"
}

func MarkerForSource(source string) string {
	switch strings.ToLower(source) {
	case "static":
		return "[STATIC]"
	case "provider", "resolved":
		return "[RESOLVED]"
	case "local":
		return "[LOCAL]"
	case "missing", "unmapped":
		return "[MISSING]"
	default:
		return "[ERROR]"
	}
}
