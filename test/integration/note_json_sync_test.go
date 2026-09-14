package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoteJSONReadCommandsIntegration(t *testing.T) {
	bin := buildCLI(t)
	itemName := "Jesse"
	project := setupProject(t,
		"DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n",
		"",
		"storage_mode: note_json\nitem_name: Jesse\n",
	)
	stub := writeRBWStubWithOptions(t, rbwStubOptions{
		Status: "unlocked",
		Items: map[string]rbwStubItem{
			itemName: {Notes: strings.TrimSpace(readRepoFile(t, "test", "testdata", "provider", "note-json-valid.json")), Password: "keep-me"},
		},
	})
	stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "sync")
	if code != 0 || stderr != "" {
		t.Fatalf("sync failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	data, err := os.ReadFile(filepath.Join(project, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "DATABASE_URL=postgres://vault/dev") || !strings.Contains(string(data), "JWT_SECRET=supersecret") {
		t.Fatalf("unexpected .env content: %s", data)
	}

	stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "diff")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "UNCHANGED ") {
		t.Fatalf("diff unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}

	stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "validate")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "CHECKED ") {
		t.Fatalf("validate unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}

	stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "missing")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "CHECKED provider") {
		t.Fatalf("missing unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
}
