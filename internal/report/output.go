package report

import (
	"fmt"
	"sort"
	"strings"
)

const (
	StatusChecked   = "CHECKED"
	StatusResolved  = "RESOLVED"
	StatusUnchanged = "UNCHANGED"
	StatusWritten   = "WRITTEN"
	StatusMissing   = "MISSING"
	StatusError     = "ERROR"
)

type Summary struct {
	Added     int
	Updated   int
	Unchanged int
	Missing   int
	Extra     int
	Error     int
	Warnings  int
}

func SummaryLine(status, target string, summary Summary, note string) string {
	parts := make([]string, 0, 6)
	if summary.Added > 0 {
		parts = append(parts, fmt.Sprintf("added: %d", summary.Added))
	}
	if summary.Updated > 0 {
		parts = append(parts, fmt.Sprintf("updated: %d", summary.Updated))
	}
	if summary.Unchanged > 0 {
		parts = append(parts, fmt.Sprintf("unchanged: %d", summary.Unchanged))
	}
	if summary.Missing > 0 {
		parts = append(parts, fmt.Sprintf("missing: %d", summary.Missing))
	}
	if summary.Extra > 0 {
		parts = append(parts, fmt.Sprintf("extra: %d", summary.Extra))
	}
	if summary.Error > 0 {
		parts = append(parts, fmt.Sprintf("error: %d", summary.Error))
	}
	if summary.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("warnings: %d", summary.Warnings))
	}
	if note != "" {
		parts = append(parts, note)
	}
	if len(parts) == 0 {
		parts = append(parts, "already up to date")
	}
	return fmt.Sprintf("%s %s (%s)", status, target, strings.Join(parts, ", "))
}

func ChangeLine(action, key, marker string) string {
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", strings.ToUpper(action), key, marker))
}

func WarningLine(key, message string) string {
	return strings.TrimSpace(fmt.Sprintf("WARNING %s %s", key, message))
}

func MissingLine(key string) string {
	return fmt.Sprintf("%s %s", StatusMissing, key)
}

func ErrorBlock(err *AppError) string {
	if err == nil {
		return ""
	}
	lines := []string{fmt.Sprintf("%s %s", StatusError, err.Code)}
	if err.Problem != "" {
		lines = append(lines, fmt.Sprintf("Problem: %s", err.Problem))
	}
	if err.Impact != "" {
		lines = append(lines, fmt.Sprintf("Impact: %s", err.Impact))
	}
	if err.Action != "" {
		lines = append(lines, fmt.Sprintf("Action: %s", err.Action))
	}
	return strings.Join(lines, "\n")
}

func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
