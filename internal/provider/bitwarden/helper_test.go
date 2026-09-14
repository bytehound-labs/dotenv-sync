package bitwarden

import (
	"os"
	"testing"

	"dotenv-sync/internal/testutil"
)

func TestMain(m *testing.M) {
	if handled, code := testutil.RunHelperProcess(); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}
