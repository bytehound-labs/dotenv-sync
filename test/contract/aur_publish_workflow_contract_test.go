package contract_test

import (
	"strings"
	"testing"
)

func TestContractAurPublishWorkflow(t *testing.T) {
	content := readRepoFile(t, ".github", "workflows", "aur-publish.yml")

	for _, want := range []string{
		"workflow_call:",
		"release:",
		"published",
		"workflow_dispatch:",
		"release_tag:",
		"pkgrel:",
		"inputs.release_tag || github.event.release.tag_name",
		"inputs.pkgrel || '1'",
		"concurrency:",
		"fetch-depth: 0",
		"persist-credentials: false",
		"actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803",
		"actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16",
		"AUR_SSH_PRIVATE_KEY",
		"gh release download",
		"git show \"${RELEASE_TAG}:LICENSE\"",
		"git show \"${RELEASE_TAG}:README.md\"",
		"--pkgrel",
		"--license-sha256",
		"--readme-sha256",
		"go run ./scripts/aurpkg",
		"ssh://aur@aur.archlinux.org/dotenv-sync-bin.git",
		"AUR_HOST_KEY: aur.archlinux.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEuBKrPzbawxA/k2g6NcyV5jmqwJ2s+zpgZGZ7tpLIcN",
		"AUR_HOST_KEY_FINGERPRINT: SHA256:RFzBCUItH9LZS0cKB5UE6ceAYhBD5C8GeOBip8Z11+4",
		"ssh-keygen -lf",
		"StrictHostKeyChecking yes",
		"UserKnownHostsFile",
		"upgpkg: dotenv-sync-bin ${PKGVER}-${PKGREL}",
		"secrets:",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("workflow missing %q", want)
		}
	}

	for _, unwanted := range []string{
		"main",
		"ssh-keyscan",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("workflow unexpectedly contains %q", unwanted)
		}
	}
}
