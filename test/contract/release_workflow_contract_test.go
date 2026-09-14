package contract_test

import (
	"regexp"
	"strings"
	"testing"
)

func TestContractReleaseWorkflow(t *testing.T) {
	content := readRepoFile(t, ".github", "workflows", "release.yml")

	for _, want := range []string{
		"push:",
		"branches:",
		"- main",
		"timeout-minutes: 30",
		"contents: write",
		"id-token: write",
		"attestations: write",
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
		"anchore/sbom-action@3ad7283483fc7af8ff2b4ea19663c2d5ca935e26",
		"format: cyclonedx-json",
		"release-sbom.cdx.json",
		"actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8",
		"subject-checksums: release-assets.sha256",
		"release-provenance.bundle.json",
		"sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6",
		"cosign-release: v3.1.3",
		"cosign sign-blob",
		"--bundle",
		"find dist -maxdepth 1 -type f -print0",
		"dist/*.sigstore.json",
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

	usesPattern := regexp.MustCompile(`(?m)^\s+uses:\s+([^\s@]+)@([^\s]+)\s*$`)
	for _, match := range usesPattern.FindAllStringSubmatch(content, -1) {
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(match[2]) {
			t.Fatalf("workflow action %q is not pinned to an immutable commit", match[1])
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
	sbomIndex := strings.Index(content, "Generate release SBOM")
	checksumIndex := strings.Index(content, "Create release asset checksums")
	attestIndex := strings.Index(content, "Attest release asset provenance")
	bundleIndex := strings.Index(content, "Save the provenance bundle for the release")
	cosignIndex := strings.Index(content, "Install Cosign")
	signIndex := strings.Index(content, "Sign every release asset")
	publishIndex := strings.Index(content, "gh release view")
	releaseIndex := strings.Index(content, "gh release create")
	aurIndex := strings.Index(content, "uses: ./.github/workflows/aur-publish.yml")
	if waitIndex == -1 || resolveIndex == -1 || testIndex == -1 || buildIndex == -1 ||
		verifyIndex == -1 || sbomIndex == -1 || checksumIndex == -1 || attestIndex == -1 ||
		bundleIndex == -1 || cosignIndex == -1 || signIndex == -1 || publishIndex == -1 ||
		releaseIndex == -1 || aurIndex == -1 {
		t.Fatal("workflow missing required ordering markers")
	}
	if waitIndex > resolveIndex || resolveIndex > testIndex || testIndex > buildIndex ||
		buildIndex > verifyIndex || verifyIndex > sbomIndex || sbomIndex > checksumIndex ||
		checksumIndex > attestIndex || attestIndex > bundleIndex || bundleIndex > cosignIndex ||
		cosignIndex > signIndex || signIndex > publishIndex || publishIndex > releaseIndex ||
		releaseIndex > aurIndex {
		t.Fatalf("workflow must run tests before creating the release")
	}
}
