package contract_test

import (
	"strings"
	"testing"
)

func TestContractSecurityWorkflows(t *testing.T) {
	tests := []struct {
		name string
		file string
		want []string
	}{
		{
			name: "CodeQL",
			file: ".github/workflows/codeql.yml",
			want: []string{
				"pull_request:",
				"schedule:",
				"security-events: write",
				"actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803",
				"github/codeql-action/init@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
				"github/codeql-action/autobuild@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
				"languages: go",
				"build-mode: autobuild",
				"github/codeql-action/analyze@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
			},
		},
		{
			name: "Gitleaks",
			file: ".github/workflows/gitleaks.yml",
			want: []string{
				"fetch-depth: 0",
				"persist-credentials: false",
				"github.com/zricethezav/gitleaks/v8@v8.28.0",
				"--log-opts=\"--all\"",
				"--redact",
			},
		},
		{
			name: "Go vulnerability scan",
			file: ".github/workflows/go-security.yml",
			want: []string{
				"Run govulncheck",
				"golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...",
				"persist-credentials: false",
			},
		},
		{
			name: "Workflow security lint",
			file: ".github/workflows/security-lint.yml",
			want: []string{
				"actionlint@v1.7.7",
				"zizmor==1.29.0",
				"--offline --collect=workflows .github/workflows",
				"persist-credentials: false",
			},
		},
		{
			name: "Dependabot",
			file: ".github/dependabot.yml",
			want: []string{
				"package-ecosystem: gomod",
				"package-ecosystem: github-actions",
				"applies-to: security-updates",
				"dependencies",
				"go",
				"ci",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := readRepoFile(t, strings.Split(test.file, "/")...)
			for _, want := range test.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s missing %q", test.file, want)
				}
			}
		})
	}
}
