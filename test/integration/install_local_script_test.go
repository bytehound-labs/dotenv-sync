package integration_test

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInstallLocalScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install-local.sh is a POSIX shell integration test")
	}
	repo := repoRoot(t)
	script := filepath.Join(repo, "scripts", "install-local.sh")
	dest := filepath.Join(t.TempDir(), "ds")

	stdout, stderr, code := runCommand(t, repo, "bash", script, "--bin", dest, "--quiet")
	if code != 0 || stderr != "" {
		t.Fatalf("install-local.sh failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	expectedVersion := expectedInstalledVersion(t, repo)
	expectedCommit := expectedShortCommit(t, repo)

	shortOut, shortErr, shortCode := runCLI(t, dest, repo, nil, "--version")
	if shortCode != 0 || shortErr != "" {
		t.Fatalf("installed --version failed: code=%d stderr=%q", shortCode, shortErr)
	}
	if strings.TrimSpace(shortOut) != "ds "+expectedVersion {
		t.Fatalf("installed short version = %q want %q", shortOut, "ds "+expectedVersion)
	}

	detailOut, detailErr, detailCode := runCLI(t, dest, repo, nil, "version")
	if detailCode != 0 || detailErr != "" {
		t.Fatalf("installed version failed: code=%d stderr=%q", detailCode, detailErr)
	}
	for _, want := range []string{"Version: " + expectedVersion, "Commit: " + expectedCommit, "Built: ", "Platform: " + currentPlatform()} {
		if !strings.Contains(detailOut, want) {
			t.Fatalf("installed version output missing %q\n%s", want, detailOut)
		}
	}
}

func expectedInstalledVersion(t *testing.T, repo string) string {
	t.Helper()
	stdout, _, code := runCommand(t, repo, "git", "describe", "--tags", "--match", "v[0-9]*.[0-9]*.[0-9]*", "--dirty", "--always")
	if code != 0 {
		return "dev"
	}
	value := strings.TrimSpace(stdout)
	if value == "" {
		return "dev"
	}
	if regexp.MustCompile(`^[0-9a-f]{7,}(-dirty)?$`).MatchString(value) {
		return "dev-" + value
	}
	return value
}

func expectedShortCommit(t *testing.T, repo string) string {
	t.Helper()
	stdout, stderr, code := runCommand(t, repo, "git", "rev-parse", "--short", "HEAD")
	if code != 0 {
		t.Fatalf("git rev-parse failed: stderr=%q", stderr)
	}
	return strings.TrimSpace(stdout)
}
