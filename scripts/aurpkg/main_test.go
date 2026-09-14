package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesAurPackageFiles(t *testing.T) {
	outputDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--version", "v1.2.3",
		"--license-sha256", strings.Repeat("c", 64),
		"--readme-sha256", strings.Repeat("d", 64),
		"--x86_64-sha256", strings.Repeat("a", 64),
		"--aarch64-sha256", strings.Repeat("b", 64),
		"--output-dir", outputDir,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), outputDir) {
		t.Fatalf("stdout = %q", stdout.String())
	}

	gotPKGBUILD, err := os.ReadFile(filepath.Join(outputDir, "PKGBUILD"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotPKGBUILD) != readGoldenFile(t, "dotenv-sync-bin.PKGBUILD") {
		t.Fatalf("PKGBUILD mismatch:\n%s", gotPKGBUILD)
	}

	gotSRCINFO, err := os.ReadFile(filepath.Join(outputDir, ".SRCINFO"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSRCINFO) != readGoldenFile(t, "dotenv-sync-bin.SRCINFO") {
		t.Fatalf(".SRCINFO mismatch:\n%s", gotSRCINFO)
	}
}

func TestRunSupportsPkgRelOverrides(t *testing.T) {
	outputDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--version", "v1.2.3",
		"--pkgrel", "2",
		"--license-sha256", strings.Repeat("c", 64),
		"--readme-sha256", strings.Repeat("d", 64),
		"--x86_64-sha256", strings.Repeat("a", 64),
		"--aarch64-sha256", strings.Repeat("b", 64),
		"--output-dir", outputDir,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d stderr=%q", code, stderr.String())
	}
	gotPKGBUILD, err := os.ReadFile(filepath.Join(outputDir, "PKGBUILD"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotPKGBUILD), "pkgrel=2") {
		t.Fatalf("PKGBUILD missing pkgrel override:\n%s", gotPKGBUILD)
	}
	gotSRCINFO, err := os.ReadFile(filepath.Join(outputDir, ".SRCINFO"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotSRCINFO), "\tpkgrel = 2\n") {
		t.Fatalf(".SRCINFO missing pkgrel override:\n%s", gotSRCINFO)
	}
}

func TestRunRejectsPositionalArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--version", "v1.2.3",
		"--license-sha256", strings.Repeat("c", 64),
		"--readme-sha256", strings.Repeat("d", 64),
		"--x86_64-sha256", strings.Repeat("a", 64),
		"--aarch64-sha256", strings.Repeat("b", 64),
		"unexpected",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected positional argument failure")
	}
	if !strings.Contains(stderr.String(), "does not accept positional arguments") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func readGoldenFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "test", "testdata", "aur", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
