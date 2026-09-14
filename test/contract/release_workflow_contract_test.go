package contract_test

import (
	"strings"
	"testing"
)

func TestContractReleaseWorkflow(t *testing.T) {
	content := readRepoFile(t, ".github", "workflows", "release.yml")

	for _, want := range []string{
		"push:",
		"branches:",
		"- main",
		"timeout-minutes: 15",
		"contents: write",
		"actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803",
		"actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16",
		"persist-credentials: false",
		"Wait for older release runs",
		"actions/workflows/release.yml/runs",
		"go test ./...",
		"go run ./scripts/nextversion --bump patch",
		"already_released",
		"Commit already released by tag",
		"stable semver tag",
		"CGO_ENABLED=0",
		"cp LICENSE README.md",
		"ds_${VERSION}_SHA256SUMS",
		"gh release view",
		"gh release upload",
		"gh release create",
		"ds --version",
		"uses: ./.github/workflows/aur-publish.yml",
		"release_tag: ${{ needs.release.outputs.version }}",
		"AUR_SSH_PRIVATE_KEY: ${{ secrets.AUR_SSH_PRIVATE_KEY }}",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("workflow missing %q", want)
		}
	}

	for _, unwanted := range []string{
		"workflow_dispatch:",
		"tags:",
		"v*.*.*",
		"concurrency:",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("workflow unexpectedly contains %q", unwanted)
		}
	}

	waitIndex := strings.Index(content, "Wait for older release runs")
	resolveIndex := strings.Index(content, "go run ./scripts/nextversion --bump patch")
	testIndex := strings.Index(content, "go test ./...")
	buildIndex := strings.Index(content, "Build release artifacts")
	verifyIndex := strings.Index(content, "ds --version")
	publishIndex := strings.Index(content, "gh release view")
	releaseIndex := strings.Index(content, "gh release create")
	aurIndex := strings.Index(content, "uses: ./.github/workflows/aur-publish.yml")
	if waitIndex == -1 || resolveIndex == -1 || testIndex == -1 || buildIndex == -1 || verifyIndex == -1 || publishIndex == -1 || releaseIndex == -1 || aurIndex == -1 {
		t.Fatal("workflow missing required ordering markers")
	}
	if waitIndex > resolveIndex || resolveIndex > testIndex || testIndex > buildIndex || buildIndex > verifyIndex || verifyIndex > publishIndex || publishIndex > releaseIndex || releaseIndex > aurIndex {
		t.Fatalf("workflow must run tests before creating the release")
	}
}
