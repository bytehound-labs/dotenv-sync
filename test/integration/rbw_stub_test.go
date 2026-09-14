package integration_test

import (
	"testing"

	"dotenv-sync/internal/testutil"
)

type rbwStubItem = testutil.RBWStubItem
type rbwStubOptions = testutil.RBWStubOptions
type rbwStub = testutil.RBWStub

func rbwLookupKey(item, field string) string {
	return item + "::" + field
}

func writeRBWStub(t *testing.T, status string, get map[string]string, missing ...string) []string {
	t.Helper()
	stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: status, Fields: get, Missing: missing})
	return stub.Env()
}

func writeRBWStubWithOptions(t *testing.T, opts rbwStubOptions) rbwStub {
	t.Helper()
	return testutil.WriteRBWStub(t, opts)
}
