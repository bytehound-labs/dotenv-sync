package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dotenv-sync/internal/testutil"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "../.."))
}

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := testBinaryPath(t, "ds")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ds")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	return bin
}

func buildCLIWithLdflags(t *testing.T, version, commit, buildTime string) string {
	t.Helper()
	bin := testBinaryPath(t, "ds")
	ldflags := fmt.Sprintf("-X dotenv-sync/pkg/dotenvsync.Version=%s -X dotenv-sync/pkg/dotenvsync.Commit=%s -X dotenv-sync/pkg/dotenvsync.BuildTime=%s", version, commit, buildTime)
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", bin, "./cmd/ds")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli with ldflags: %v\n%s", err, out)
	}
	return bin
}

func runCLI(t *testing.T, bin, dir string, extraEnv []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout.String(), stderr.String(), exitErr.ExitCode()
	}
	t.Fatalf("run cli: %v", err)
	return "", "", 0
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func readGoldenFile(t *testing.T, name string) string {
	t.Helper()
	return readRepoFile(t, "test", "testdata", "golden", name)
}

func readReleaseFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	content := readRepoFile(t, "test", "testdata", "release", name)
	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func renderTemplate(input string, replacements map[string]string) string {
	output := input
	for old, newValue := range replacements {
		output = strings.ReplaceAll(output, old, newValue)
	}
	return output
}

func compactJSON(t *testing.T, input string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(input)); err != nil {
		t.Fatalf("compact json: %v", err)
	}
	return buf.String()
}

func currentPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func testBinaryPath(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(t.TempDir(), name)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupProject(t *testing.T, schema, env, cfg string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env.example"), schema)
	if env != "" {
		writeFile(t, filepath.Join(dir, ".env"), env)
	}
	if cfg != "" {
		writeFile(t, filepath.Join(dir, ".envsync.yaml"), cfg)
	}
	return dir
}

func rbwLookupKey(item, field string) string {
	return item + "::" + field
}

type rbwStubItem = testutil.RBWStubItem
type rbwStubOptions = testutil.RBWStubOptions
type rbwStub = testutil.RBWStub

func writeRBWStub(t *testing.T, status string, get map[string]string, missing ...string) (string, []string) {
	t.Helper()
	stub := writeRBWStubWithOptions(t, rbwStubOptions{Status: status, Fields: get, Missing: missing})
	return stub.Path(), stub.Env()
}

func writeRBWStubWithOptions(t *testing.T, opts rbwStubOptions) rbwStub {
	t.Helper()
	return testutil.WriteRBWStub(t, opts)
}
