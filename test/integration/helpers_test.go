package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootFromTB(tb testing.TB) string {
	tb.Helper()
	wd, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "../.."))
}

func repoRoot(t *testing.T) string {
	return repoRootFromTB(t)
}

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := testBinaryPath(t, "ds")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ds")
	cmd.Dir = repoRootFromTB(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	return bin
}

func buildCLIWithLdflags(t *testing.T, version, commit, buildTime string) string {
	t.Helper()
	bin := testBinaryPath(t, "ds")
	ldflags := "-X dotenv-sync/pkg/dotenvsync.Version=" + version +
		" -X dotenv-sync/pkg/dotenvsync.Commit=" + commit +
		" -X dotenv-sync/pkg/dotenvsync.BuildTime=" + buildTime
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", bin, "./cmd/ds")
	cmd.Dir = repoRootFromTB(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli with ldflags: %v\n%s", err, out)
	}
	return bin
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRootFromTB(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
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

func runGoMain(t *testing.T, dir, pkg string, args ...string) (string, string, int) {
	t.Helper()
	bin := testBinaryPath(t, "go-main")
	build := exec.Command("go", "build", "-o", bin, pkg)
	build.Dir = dir
	build.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go main: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=")
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
	t.Fatalf("run go main: %v", err)
	return "", "", 0
}

func runCommand(t *testing.T, dir, name string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
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
	t.Fatalf("run command: %v", err)
	return "", "", 0
}

func readGoldenFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRootFromTB(t), "test", "testdata", "golden", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readReleaseFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	path := filepath.Join(repoRootFromTB(t), "test", "testdata", "release", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
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

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	commands := [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	}
	for _, args := range commands {
		if stdout, stderr, code := runCommand(t, dir, "git", args...); code != 0 {
			t.Fatalf("git %v failed: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	if stdout, stderr, code := runCommand(t, dir, "git", "add", "."); code != 0 {
		t.Fatalf("git add failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if stdout, stderr, code := runCommand(t, dir, "git", "commit", "-m", message); code != 0 {
		t.Fatalf("git commit failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code := runCommand(t, dir, "git", "rev-parse", "--short", "HEAD")
	if code != 0 {
		t.Fatalf("git rev-parse failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	return strings.TrimSpace(stdout)
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
