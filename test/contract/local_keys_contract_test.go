package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractLocalKeys(t *testing.T) {
	bin := buildCLI(t)
	configYAML := "item_name: local-demo\nlocal_keys:\n  - FILE\n"

	t.Run("preserves different host values without provider access", func(t *testing.T) {
		project := setupProject(t, "FILE=\nPORT=8080\n", "FILE=1\nPORT=8080\n", configYAML)

		stdout, stderr, code := runCLI(t, bin, project, nil, "sync")
		if code != 0 || stderr != "" {
			t.Fatalf("host one sync failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		data, err := os.ReadFile(filepath.Join(project, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "FILE=1\nPORT=8080\n" {
			t.Fatalf("host one value changed: %q", data)
		}

		writeFile(t, filepath.Join(project, ".env"), "FILE=2\nPORT=8080\n")
		stdout, stderr, code = runCLI(t, bin, project, nil, "sync")
		if code != 0 || stderr != "" {
			t.Fatalf("host two sync failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		data, err = os.ReadFile(filepath.Join(project, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "FILE=2\nPORT=8080\n" {
			t.Fatalf("host two value changed: %q", data)
		}
		if strings.Contains(stdout, "FILE=2") {
			t.Fatalf("local value leaked in output: %s", stdout)
		}
	})

	t.Run("missing local value warns but succeeds across commands", func(t *testing.T) {
		project := setupProject(t, "FILE=\n", "", configYAML)

		stdout, stderr, code := runCLI(t, bin, project, nil, "sync")
		if code != 0 || stderr != "" {
			t.Fatalf("sync warning failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "WARNING FILE [LOCAL] local value is missing or blank; set it in .env") {
			t.Fatalf("sync warning missing: %s", stdout)
		}
		data, err := os.ReadFile(filepath.Join(project, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "FILE=\n" {
			t.Fatalf("expected blank local assignment, got %q", data)
		}

		stdout, stderr, code = runCLI(t, bin, project, nil, "diff")
		if code != 0 || stderr != "" || !strings.Contains(stdout, "WARNING FILE [LOCAL]") {
			t.Fatalf("diff warning unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}

		stdout, stderr, code = runCLI(t, bin, project, nil, "validate")
		if code != 0 || stderr != "" || !strings.Contains(stdout, "WARNING FILE [LOCAL]") {
			t.Fatalf("validate warning unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}

		stdout, stderr, code = runCLI(t, bin, project, nil, "missing")
		if code != 0 || stderr != "" {
			t.Fatalf("missing local warning unexpected: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if strings.Contains(stdout, "WARNING") || !strings.Contains(stdout, "CHECKED provider") {
			t.Fatalf("missing should remain provider-only: %s", stdout)
		}
	})

	t.Run("mixed provider and local keys only resolve provider keys", func(t *testing.T) {
		project := setupProject(t,
			"FILE=\nDATABASE_URL=\n",
			"FILE=1\n",
			"item_name: local-demo\nlocal_keys:\n  - FILE\nmapping:\n  DATABASE_URL: database-url\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{
			Status: "unlocked",
			Fields: map[string]string{
				"local-demo::database-url": "postgres://vault/dev",
			},
		})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "sync")
		if code != 0 || stderr != "" {
			t.Fatalf("mixed sync failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if strings.Contains(stub.Log(t), "FILE") {
			t.Fatalf("provider was asked for local key: %s", stub.Log(t))
		}
		data, err := os.ReadFile(filepath.Join(project, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "FILE=1") || !strings.Contains(string(data), "DATABASE_URL=postgres://vault/dev") {
			t.Fatalf("mixed sync output unexpected: %q", data)
		}
	})

	t.Run("push excludes local values and removes stale note_json copies", func(t *testing.T) {
		project := setupProject(t,
			"FILE=\nDATABASE_URL=\n",
			"FILE=1\nDATABASE_URL=postgres://vault/dev\n",
			"storage_mode: note_json\nitem_name: local-demo\nlocal_keys:\n  - FILE\n",
		)
		stub := writeRBWStubWithOptions(t, rbwStubOptions{
			Status: "unlocked",
			Items: map[string]rbwStubItem{
				"local-demo": {
					Notes: `{"format":"dotenv-sync/note-json@v1","env":{"FILE":"old-host","DATABASE_URL":"old"}}`,
				},
			},
		})

		stdout, stderr, code := runCLI(t, bin, project, stub.Env(), "push")
		if code != 0 || stderr != "" {
			t.Fatalf("local push failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		notes := stub.Note(t, "local-demo")
		if strings.Contains(notes, `"FILE"`) {
			t.Fatalf("local key remained in provider notes: %s", notes)
		}
		if !strings.Contains(notes, `"DATABASE_URL":"postgres://vault/dev"`) {
			t.Fatalf("provider key missing from provider notes: %s", notes)
		}
		if strings.Contains(stdout, "postgres://vault/dev") || strings.Contains(stdout, "old-host") {
			t.Fatalf("push output leaked a value: %s", stdout)
		}
	})

	t.Run("init and reverse keep configured local values blank in schema", func(t *testing.T) {
		initProject := setupProject(t, "", "FILE=1\nPORT=8080\n", configYAML)
		stdout, stderr, code := runCLI(t, bin, initProject, nil, "init", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("init local dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "ADD FILE [LOCAL]") {
			t.Fatalf("init did not classify local key: %s", stdout)
		}
		_, _, code = runCLI(t, bin, initProject, nil, "init")
		if code != 0 {
			t.Fatalf("init local write failed: code=%d", code)
		}
		data, err := os.ReadFile(filepath.Join(initProject, ".env.example"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "FILE=\nPORT=8080\n" {
			t.Fatalf("init wrote unexpected schema: %q", data)
		}

		reverseProject := setupProject(t, "PORT=8080\n", "PORT=8080\nFILE=2\n", configYAML)
		stdout, stderr, code = runCLI(t, bin, reverseProject, nil, "reverse")
		if code != 0 || stderr != "" {
			t.Fatalf("reverse local failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "ADD FILE [LOCAL]") {
			t.Fatalf("reverse did not classify local key: %s", stdout)
		}
		data, err = os.ReadFile(filepath.Join(reverseProject, ".env.example"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "PORT=8080\nFILE=\n" {
			t.Fatalf("reverse wrote unexpected schema: %q", data)
		}
	})

	t.Run("scaffold excludes configured local keys", func(t *testing.T) {
		project := setupProject(t,
			"FILE=\nJWT_SECRET=\n",
			"",
			"provider: keepass\nlocal_keys:\n  - FILE\nkeepass_database: test-secrets.kdbx\nkeepass_group: dotenv\n",
		)
		stdout, stderr, code := runCLI(t, bin, project, nil, "scaffold", "--dry-run")
		if code != 0 || stderr != "" {
			t.Fatalf("scaffold local dry-run failed: code=%d stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stdout, "ADD JWT_SECRET [DRY-RUN]") || strings.Contains(stdout, "FILE") {
			t.Fatalf("scaffold included local key: %s", stdout)
		}
	})
}
