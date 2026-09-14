package bitwarden

import (
	"context"
	"path/filepath"
	"testing"

	"dotenv-sync/internal/testutil"
)

func TestCheckReadinessWithMissingBinary(t *testing.T) {
	status, err := checkReadinessWithClient(context.Background(), &RBWClient{Bin: filepath.Join(t.TempDir(), "rbw-missing")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Code != "E001" {
		t.Fatalf("expected E001, got %+v", status)
	}
}

func TestCheckReadinessWithStubStatuses(t *testing.T) {
	cases := []struct {
		name string
		opts testutil.RBWStubOptions
		code string
	}{
		{
			name: "unlocked",
			opts: testutil.RBWStubOptions{Status: "unlocked"},
			code: "",
		},
		{
			name: "locked",
			opts: testutil.RBWStubOptions{Status: "locked"},
			code: "E003",
		},
		{
			name: "logged-out",
			opts: testutil.RBWStubOptions{Status: "logged out"},
			code: "E002",
		},
		{
			name: "legacy-status-fallback",
			opts: testutil.RBWStubOptions{Status: "unlocked", LegacyStatusFallback: true},
			code: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := testutil.WriteRBWStub(t, tc.opts)
			stub.SetEnv(t)
			status, err := checkReadinessWithClient(context.Background(), &RBWClient{Bin: stub.Path()})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status.Code != tc.code {
				t.Fatalf("expected code %q, got %+v", tc.code, status)
			}
		})
	}
}
