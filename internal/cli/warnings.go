package cli

import (
	"fmt"
	"io"

	"dotenv-sync/internal/report"
	syncpkg "dotenv-sync/internal/sync"
)

func printWarnings(w io.Writer, warnings []syncpkg.ValidationIssue) {
	for _, warning := range warnings {
		fmt.Fprintln(w, report.WarningLine(warning.Key, report.MarkerForSource("local")+" "+warning.Message))
	}
}
