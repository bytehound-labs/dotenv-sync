package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if handled, code := RunHelperProcess(); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestRBWStubCoversCommandsAndMutations(t *testing.T) {
	stub := WriteRBWStub(t, RBWStubOptions{
		Status: "unlocked",
		Fields: map[string]string{
			"repo::field": "field-value",
		},
		Missing: []string{"repo::missing"},
		Items: map[string]RBWStubItem{
			"repo": {Notes: "old-notes", Password: "old-password"},
		},
	})
	stub.SetEnv(t)

	if stdout, code := runHelperCommand(t, stub.Env(), stub.Path(), "unlocked"); code != 0 || stdout != "" {
		t.Fatalf("unlocked command: code=%d stdout=%q", code, stdout)
	}
	if stdout, code := runHelperCommand(t, stub.Env(), stub.Path(), "list"); code != 0 || !strings.Contains(stdout, "DATABASE_URL") {
		t.Fatalf("list command: code=%d stdout=%q", code, stdout)
	}
	if stdout, code := runHelperCommand(t, stub.Env(), stub.Path(), "get", "--field", "field", "repo"); code != 0 || strings.TrimSpace(stdout) != "field-value" {
		t.Fatalf("field command: code=%d stdout=%q", code, stdout)
	}
	if stdout, code := runHelperCommand(t, stub.Env(), stub.Path(), "get", "--field", "password", "repo"); code != 0 || strings.TrimSpace(stdout) != "old-password" {
		t.Fatalf("password command: code=%d stdout=%q", code, stdout)
	}
	raw, code := runHelperCommand(t, stub.Env(), stub.Path(), "get", "--raw", "repo")
	if code != 0 {
		t.Fatalf("raw command: code=%d stdout=%q", code, raw)
	}
	var payload struct {
		Notes string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload.Notes != "old-notes" {
		t.Fatalf("raw payload: err=%v payload=%q", err, raw)
	}
	if _, code := runHelperCommand(t, stub.Env(), stub.Path(), "get", "--field", "missing", "repo"); code == 0 {
		t.Fatal("missing field unexpectedly succeeded")
	}

	source := t.TempDir() + string(os.PathSeparator) + "editor.txt"
	if err := os.WriteFile(source, []byte("new-password\n\nnew-notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DS_RBW_EDITOR_SOURCE", source)
	if _, code := runHelperCommand(t, stub.Env(), stub.Path(), "edit", "repo"); code != 0 {
		t.Fatal("edit command failed")
	}
	if got := stub.Password(t, "repo"); got != "new-password" {
		t.Fatalf("updated password = %q", got)
	}
	if got := stub.Note(t, "repo"); got != "new-notes" {
		t.Fatalf("updated notes = %q", got)
	}
	if _, code := runHelperCommand(t, stub.Env(), stub.Path(), "add", "new-item"); code != 0 {
		t.Fatal("add command failed")
	}
	if _, code := runHelperCommand(t, stub.Env(), stub.Path(), "sync"); code != 0 {
		t.Fatal("sync command failed")
	}
	if !strings.Contains(stub.Log(t), "get --raw repo") {
		t.Fatalf("helper log missing raw lookup: %s", stub.Log(t))
	}
}

func TestKeepassStubCoversCommands(t *testing.T) {
	stub := WriteKeepassStub(t, KeepassStubOptions{
		Entries: map[string][]string{"dotenv": {"DATABASE_URL", "subgroup/"}},
		Values:  map[string]string{"dotenv/DATABASE_URL": "postgres://vault/dev"},
		Missing: []string{"dotenv/MISSING", "missing-group"},
	})
	stub.SetEnv(t)

	stdout, code := runHelperCommand(t, stub.Env(), stub.Path(), "show", "-s", "test.kdbx", "dotenv/DATABASE_URL")
	if code != 0 || !strings.Contains(stdout, "Password: postgres://vault/dev") {
		t.Fatalf("show command: code=%d stdout=%q", code, stdout)
	}
	stdout, code = runHelperCommand(t, stub.Env(), stub.Path(), "ls", "test.kdbx", "dotenv")
	if code != 0 || !strings.Contains(stdout, "DATABASE_URL") {
		t.Fatalf("ls command: code=%d stdout=%q", code, stdout)
	}
	if _, code = runHelperCommand(t, stub.Env(), stub.Path(), "show", "-s", "test.kdbx", "dotenv/MISSING"); code == 0 {
		t.Fatal("missing entry unexpectedly succeeded")
	}
	if _, code = runHelperCommand(t, stub.Env(), stub.Path(), "ls", "test.kdbx", "missing-group"); code == 0 {
		t.Fatal("missing group unexpectedly succeeded")
	}
	if _, code = runHelperCommand(t, stub.Env(), stub.Path(), "add", "-p", "test.kdbx", "dotenv/NEW"); code != 0 {
		t.Fatal("add command failed")
	}
}

func runHelperCommand(t *testing.T, env []string, path string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout.String() + stderr.String(), exitErr.ExitCode()
	}
	t.Fatalf("run helper: %v", err)
	return "", 0
}
