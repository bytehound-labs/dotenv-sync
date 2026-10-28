package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushCommandIntegration(t *testing.T) {
	bin := buildCLI(t)
	itemName := "Jesse"
	configYAML := "storage_mode: note_json\nitem_name: Jesse\n"

	t.Run("note_json pushes retain keys across branch schemas", func(t *testing.T) {
		branchA := setupProject(t,
			"COMMON=\nBRANCH_A_ONLY=\n",
			"COMMON=shared-local\nBRANCH_A_ONLY=branch-a-local\n",
			configYAML,
		)
		branchB := setupProject(t,
			"COMMON=\nBRANCH_B_ONLY=\n",
			"COMMON=shared-local\nBRANCH_B_ONLY=branch-b-local\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, branchA, stub.Env(), "push")
		if code != 0 || stderr != "" || !strings.Contains(stdout, "WRITTEN bitwarden:Jesse") {
			t.Fatalf("branch A push failed: code=%d stderr=%q", code, stderr)
		}

		logBeforeDryRun := stub.Log(t)
		stdout, stderr, code = runCLI(t, bin, branchB, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("branch B dry-run failed: code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{
			"ADD BRANCH_B_ONLY [REDACTED]",
			"UNCHANGED BRANCH_A_ONLY [REDACTED]",
			"CHECKED bitwarden:Jesse (added: 1, unchanged: 2, dry-run)",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("branch B dry-run missing %q", want)
			}
		}
		if strings.Count(stub.Log(t), "add Jesse")+strings.Count(stub.Log(t), "edit Jesse") !=
			strings.Count(logBeforeDryRun, "add Jesse")+strings.Count(logBeforeDryRun, "edit Jesse") {
			t.Fatal("branch B dry-run mutated the provider")
		}

		stdout, stderr, code = runCLI(t, bin, branchB, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("branch B push failed: code=%d stderr=%q", code, stderr)
		}
		wantNotes := `{"format":"dotenv-sync/note-json@v1","env":{"BRANCH_A_ONLY":"branch-a-local","BRANCH_B_ONLY":"branch-b-local","COMMON":"shared-local"}}`
		if notes := compactJSON(t, strings.TrimSpace(stub.Note(t, itemName))); notes != wantNotes {
			t.Fatal("branch B push did not preserve the union of branch keys")
		}

		for _, branch := range []struct {
			name       string
			dir        string
			ownKey     string
			foreignKey string
		}{
			{name: "A", dir: branchA, ownKey: "BRANCH_A_ONLY", foreignKey: "BRANCH_B_ONLY"},
			{name: "B", dir: branchB, ownKey: "BRANCH_B_ONLY", foreignKey: "BRANCH_A_ONLY"},
		} {
			stdout, stderr, code = runCLI(t, bin, branch.dir, stub.Env(), "sync")
			if code != 0 || stderr != "" {
				t.Fatalf("branch %s sync failed: code=%d stderr=%q", branch.name, code, stderr)
			}
			data, err := os.ReadFile(filepath.Join(branch.dir, ".env"))
			if err != nil {
				t.Fatalf("could not read branch %s env file", branch.name)
			}
			if !strings.Contains(string(data), branch.ownKey+"=") || strings.Contains(string(data), branch.foreignKey+"=") {
				t.Fatalf("branch %s sync did not keep only its schema keys", branch.name)
			}
		}
	})

	t.Run("dry run create unchanged and update flows", func(t *testing.T) {
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
		wantDryRun := renderTemplate(readRepoFile(t, "test", "testdata", "golden", "push-dry-run.txt"), map[string]string{"{{ITEM}}": itemName})
		if stdout != wantDryRun {
			t.Fatalf("push dry-run stdout = %q want %q", stdout, wantDryRun)
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("dry-run should not mutate provider, log=%q", log)
		}

		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push create failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (added: 3)") {
			t.Fatalf("unexpected create summary: %s", stdout)
		}
		wantNotes := compactJSON(t, strings.TrimSpace(readRepoFile(t, "test", "testdata", "provider", "note-json-valid.json")))
		if notes := compactJSON(t, strings.TrimSpace(stub.Note(t, itemName))); notes != wantNotes {
			t.Fatalf("unexpected created notes = %q want %q", notes, wantNotes)
		}
		if got := strings.Count(stub.Log(t), "add Jesse"); got != 1 {
			t.Fatalf("expected one add, got %d log=%q", got, stub.Log(t))
		}
		if got := strings.Count(stub.Log(t), "sync"); got != 1 {
			t.Fatalf("expected one sync, got %d log=%q", got, stub.Log(t))
		}

		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push unchanged failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		wantUnchanged := renderTemplate(readRepoFile(t, "test", "testdata", "golden", "push-unchanged.txt"), map[string]string{"{{ITEM}}": itemName})
		if stdout != wantUnchanged {
			t.Fatalf("push unchanged stdout = %q want %q", stdout, wantUnchanged)
		}
		if got := strings.Count(stub.Log(t), "add Jesse"); got != 1 {
			t.Fatalf("unchanged run should not add again, log=%q", stub.Log(t))
		}
		if got := strings.Count(stub.Log(t), "edit Jesse"); got != 0 {
			t.Fatalf("unchanged run should not edit, log=%q", stub.Log(t))
		}

		writeFile(t, filepath.Join(project, ".env"), "DATABASE_URL=postgres://vault/dev\nJWT_SECRET=rotated\nPORT=8080\n")
		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("push update failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (updated: 1, unchanged: 2)") {
			t.Fatalf("unexpected update summary: %s", stdout)
		}
		if !strings.Contains(stub.Note(t, itemName), "\"JWT_SECRET\":\"rotated\"") {
			t.Fatalf("expected rotated secret in notes, got %q", stub.Note(t, itemName))
		}
		if got := strings.Count(stub.Log(t), "edit Jesse"); got != 1 {
			t.Fatalf("expected one edit, got %d log=%q", got, stub.Log(t))
		}
		if got := strings.Count(stub.Log(t), "sync"); got != 2 {
			t.Fatalf("expected second sync after update, got %d log=%q", got, stub.Log(t))
		}
	})

	t.Run("fields mode password mapping create unchanged and update flows", func(t *testing.T) {
		project := setupProject(t,
			"DB_PASSWD=\nAPP_MODE=repo1\n",
			"DB_PASSWD=shared-v1\nAPP_MODE=repo1\n",
			"item_name: Jesse\nmapping:\n  DB_PASSWD: password\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("fields push dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "ADD DB_PASSWD [REDACTED]") || !strings.Contains(stdout, "CHECKED bitwarden:Jesse (added: 1, dry-run)") {
			t.Fatalf("unexpected fields dry-run stdout: %s", stdout)
		}
		if log := stub.Log(t); strings.Contains(log, "add ") || strings.Contains(log, "edit ") || strings.Contains(log, "sync") {
			t.Fatalf("dry-run should not mutate provider, log=%q", log)
		}

		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("fields push create failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (added: 1)") {
			t.Fatalf("unexpected fields create summary: %s", stdout)
		}
		if got := stub.Password(t, itemName); got != "shared-v1" {
			t.Fatalf("unexpected created password %q", got)
		}
		if got := stub.Note(t, itemName); got != "" {
			t.Fatalf("expected blank notes for fields mode, got %q", got)
		}
		if got := strings.Count(stub.Log(t), "add Jesse"); got != 1 {
			t.Fatalf("expected one add, got %d log=%q", got, stub.Log(t))
		}
		if got := strings.Count(stub.Log(t), "sync"); got != 1 {
			t.Fatalf("expected one sync, got %d log=%q", got, stub.Log(t))
		}

		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("fields push unchanged failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "UNCHANGED bitwarden:Jesse (unchanged: 1, already up to date)") {
			t.Fatalf("unexpected fields unchanged summary: %s", stdout)
		}
		if got := strings.Count(stub.Log(t), "edit Jesse"); got != 0 {
			t.Fatalf("unchanged run should not edit, log=%q", stub.Log(t))
		}

		writeFile(t, filepath.Join(project, ".env"), "DB_PASSWD=shared-v2\nAPP_MODE=repo1\n")
		stdout, stderr, code = runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("fields push update failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WRITTEN bitwarden:Jesse (updated: 1)") {
			t.Fatalf("unexpected fields update summary: %s", stdout)
		}
		if got := stub.Password(t, itemName); got != "shared-v2" {
			t.Fatalf("expected updated password, got %q", got)
		}
		if got := strings.Count(stub.Log(t), "edit Jesse"); got != 1 {
			t.Fatalf("expected one edit, got %d log=%q", got, stub.Log(t))
		}
	})

	t.Run("unsupported fields mode mapping is rejected", func(t *testing.T) {
		project := setupProject(t,
			"DB_PASSWD=\n",
			"DB_PASSWD=shared-v1\n",
			"item_name: Jesse\nmapping:\n  DB_PASSWD: shared_password\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push")
		if code != 1 || !strings.Contains(stderr, "ERROR E011") {
			t.Fatalf("expected unsupported fields-mode error, code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if strings.Contains(stub.Log(t), "add ") || strings.Contains(stub.Log(t), "edit ") || strings.Contains(stub.Log(t), "sync") {
			t.Fatalf("expected no provider mutation, log=%q", stub.Log(t))
		}
	})

	t.Run("extra previews keep schema unchanged", func(t *testing.T) {
		project := setupProject(t,
			"DATABASE_URL=\n",
			"DATABASE_URL=postgres://vault/dev\nEXTRA_FLAG=true\n",
			configYAML,
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: "unlocked"})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("push extra preview failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
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
