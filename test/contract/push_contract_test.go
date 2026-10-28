package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractPush(t *testing.T) {
	bin := buildCLI(t)
	itemName := "Jesse"
	configYAML := "storage_mode: note_json\nitem_name: Jesse\n"
	t.Run("dry run previews redacted changes", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n",
			"DATABASE_URL=postgres://vault/dev\nJWT_SECRET=supersecret\nPORT=8080\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("push dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		want := renderTemplate(readRepoFile(t, "test", "testdata", "golden", "push-dry-run.txt"), map[string]string{"{{ITEM}}": itemName})
		if stdout != want {
			t.Fatalf("push dry-run stdout = %q want %q", stdout, want)
		}
		if strings.Contains(stdout, "supersecret") {
			t.Fatalf("push dry-run leaked secret: %s", stdout)
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("dry-run should not mutate provider, log=%q", log)
		}
	})

	t.Run("dry run distinguishes retained keys from stale local keys", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\nFILE=\n",
			"DATABASE_URL=synthetic-db\nFILE=host-local\n",
			"storage_mode: note_json\nitem_name: Jesse\nlocal_keys:\n  - FILE\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{
			Status: "unlocked",
			Items: map[string]rbwStubItem{
				itemName: {
					Notes: `{"format":"dotenv-sync/note-json@v1","env":{"DATABASE_URL":"synthetic-db","FILE":"stale-local-copy","OTHER_BRANCH_KEY":"branch-value"}}`,
				},
			},
		})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("push dry-run failed: code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{
			"UNCHANGED DATABASE_URL [REDACTED]",
			"UPDATE FILE [EMPTY]",
			"UNCHANGED OTHER_BRANCH_KEY [REDACTED]",
			"CHECKED bitwarden:Jesse (updated: 1, unchanged: 2, dry-run)",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("push dry-run missing %q", want)
			}
		}
		for _, secret := range []string{"synthetic-db", "stale-local-copy", "branch-value"} {
			if strings.Contains(stdout, secret) {
				t.Fatal("push dry-run exposed a provider or local value")
			}
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("dry-run should not mutate provider, log=%q", log)
		}
	})

	t.Run("create writes repo-scoped note_json payload", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n",
			"DATABASE_URL=postgres://vault/dev\nJWT_SECRET=supersecret\nPORT=8080\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (added: 3)") {
			t.Fatalf("unexpected push summary: %s", stdout)
		}
		notes := compactJSON(t, strings.TrimSpace(stub.Note(t, itemName)))
		wantNotes := compactJSON(t, strings.TrimSpace(readRepoFile(t, "test", "testdata", "provider", "note-json-valid.json")))
		if notes != wantNotes {
			t.Fatalf("unexpected provider notes = %q want %q", notes, wantNotes)
		}
		if got := stub.Password(t, itemName); got != "" {
			t.Fatalf("expected blank password, got %q", got)
		}
		log := stub.Log(t)
		if !strings.Contains(log, "add Jesse") || !strings.Contains(log, "sync") {
			t.Fatalf("expected add + sync in log, got %q", log)
		}
	})

	t.Run("unchanged skips provider mutation", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n",
			"DATABASE_URL=postgres://vault/dev\nJWT_SECRET=supersecret\nPORT=8080\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{
			Status: "unlocked",
			Items: map[string]rbwStubItem{
				itemName: {Notes: strings.TrimSpace(readRepoFile(t, "test", "testdata", "provider", "note-json-valid.json")), Password: "keep-me"},
			},
		})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push unchanged failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		want := renderTemplate(readRepoFile(t, "test", "testdata", "golden", "push-unchanged.txt"), map[string]string{"{{ITEM}}": itemName})
		if stdout != want {
			t.Fatalf("push unchanged stdout = %q want %q", stdout, want)
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("unchanged run should not mutate provider, log=%q", log)
		}
	})

	t.Run("fields mode password mapping writes the shared password field", func(t *testing.T) {
		project := setupProject(t,
			"DB_PASSWD=\nAPP_MODE=repo1\n",
			"DB_PASSWD=shared-v1\nAPP_MODE=repo1\n",
			"item_name: Jesse\nmapping:\n  DB_PASSWD: password\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("push fields dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		for _, want := range []string{"ADD DB_PASSWD [REDACTED]", "CHECKED bitwarden:Jesse (added: 1, dry-run)"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("push fields dry-run missing %q\n%s", want, stdout)
			}
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("dry-run should not mutate provider, log=%q", log)
		}

		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push fields write failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (added: 1)") {
			t.Fatalf("unexpected fields push summary: %s", stdout)
		}
		if got := stub.Password(t, itemName); got != "shared-v1" {
			t.Fatalf("expected shared password to be written, got %q", got)
		}
		if got := stub.Note(t, itemName); got != "" {
			t.Fatalf("expected blank notes in fields mode, got %q", got)
		}
	})

	t.Run("unsupported fields mode mapping explains the safe fallback", func(t *testing.T) {
		project := setupProject(t,
			"DB_PASSWD=\n",
			"DB_PASSWD=shared-v1\n",
			"item_name: Jesse\nmapping:\n  DB_PASSWD: shared_password\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push")
		if code != 1 {
			t.Fatalf("push unsupported fields mode exit code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		want := readRepoFile(t, "test", "testdata", "golden", "push-fields-mode-error.txt")
		if stderr != want {
			t.Fatalf("push unsupported fields stderr = %q want %q", stderr, want)
		}
		if stdout != "" {
			t.Fatalf("expected empty stdout, got %q", stdout)
		}
		if strings.Contains(stub.Log(t), "add ") || strings.Contains(stub.Log(t), "edit ") || strings.Contains(stub.Log(t), "sync") {
			t.Fatalf("expected no provider mutation in unsupported fields mode, log=%q", stub.Log(t))
		}
	})

	t.Run("extra env keys stay redacted and do not touch schema", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\n",
			"DATABASE_URL=postgres://vault/dev\nEXTRA_FLAG=true\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("push extra dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "EXTRA EXTRA_FLAG [REDACTED]") {
			t.Fatalf("expected extra preview, got %s", stdout)
		}
		data, err := os.ReadFile(filepath.Join(project, ".env.example"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "DATABASE_URL=\n" {
			t.Fatalf("schema should remain unchanged, got %q", string(data))
		}
	})
}
