package keepass

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/testutil"
)

func adapterWithStub(t *testing.T, stub testutil.KeepassStub, dbPath, group, password string) *Adapter {
	t.Helper()
	stub.SetEnv(t)
	cfg := config.Config{
		KeePassDatabase: dbPath,
		KeePassGroup:    group,
	}
	a := NewAdapter(cfg)
	a.client.Bin = stub.Path()
	a.client.Password = password
	return a
}

func TestAdapterNameIsKeepass(t *testing.T) {
	a := NewAdapter(config.Config{})
	if a.Name() != "keepass" {
		t.Fatalf("expected name=keepass, got %q", a.Name())
	}
}

func TestAdapterResolveReturnsValue(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Values: map[string]string{"dotenv/DATABASE_URL": "postgres://vault/dev"},
	})
	a := adapterWithStub(t, stub, dbPath, "dotenv", "masterpassword")
	res, err := a.Resolve(context.Background(), "DATABASE_URL", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Source != "provider" || res.Value != "postgres://vault/dev" {
		t.Fatalf("unexpected resolution: %+v", res)
	}
}

func TestAdapterResolveUsesCacheOnSecondCall(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Values: map[string]string{"dotenv/MY_KEY": "somevalue"},
	})
	a := adapterWithStub(t, stub, dbPath, "dotenv", "pw")

	if _, err := a.Resolve(context.Background(), "MY_KEY", ""); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := a.Resolve(context.Background(), "MY_KEY", ""); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
}

func TestAdapterResolveMissingKeyReturnsE005(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Missing: []string{"dotenv/MISSING_KEY"},
	})
	a := adapterWithStub(t, stub, dbPath, "dotenv", "pw")
	res, err := a.Resolve(context.Background(), "MISSING_KEY", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Source != "missing" || res.IssueCode != "E005" {
		t.Fatalf("expected missing/E005, got %+v", res)
	}
}

func TestAdapterResolveManyReturnsAllResolutions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Values: map[string]string{
			"dotenv/DATABASE_URL": "postgres://vault/dev",
			"dotenv/JWT_SECRET":   "topsecret",
		},
	})
	a := adapterWithStub(t, stub, dbPath, "dotenv", "pw")
	results, err := a.ResolveMany(context.Background(), map[string]string{
		"DATABASE_URL": "",
		"JWT_SECRET":   "",
	})
	if err != nil {
		t.Fatalf("ResolveMany: %v", err)
	}
	if results["DATABASE_URL"].Value != "postgres://vault/dev" {
		t.Fatalf("unexpected DATABASE_URL: %+v", results["DATABASE_URL"])
	}
	if results["JWT_SECRET"].Value != "topsecret" {
		t.Fatalf("unexpected JWT_SECRET: %+v", results["JWT_SECRET"])
	}
}

func TestAdapterCheckReadinessBinaryMissing(t *testing.T) {
	a := NewAdapter(config.Config{
		KeePassDatabase: "/some/db.kdbx",
		KeePassGroup:    "dotenv",
	})
	a.client.Bin = "/nonexistent/keepassxc-cli"

	status, err := a.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CLIInstalled {
		t.Fatal("expected CLIInstalled=false")
	}
	if status.Code != "E001" {
		t.Fatalf("expected E001, got %q", status.Code)
	}
}

func TestAdapterCheckReadinessDatabaseMissing(t *testing.T) {
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{})
	a := NewAdapter(config.Config{
		KeePassDatabase: "/nonexistent/db.kdbx",
		KeePassGroup:    "dotenv",
	})
	stub.SetEnv(t)
	a.client.Bin = stub.Path()
	a.client.Password = "pw"

	status, err := a.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Code != "E002" {
		t.Fatalf("expected E002 for missing db, got %q", status.Code)
	}
}

func TestAdapterCheckReadinessGroupMissing(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Missing: []string{"nosuchgroup"},
	})
	a := adapterWithStub(t, stub, dbPath, "nosuchgroup", "pw")

	status, err := a.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Code != "E004" {
		t.Fatalf("expected E004 for missing group, got %q", status.Code)
	}
}

func TestAdapterCheckReadinessReady(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.kdbx")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Entries: map[string][]string{"dotenv": {"DATABASE_URL"}},
	})
	a := adapterWithStub(t, stub, dbPath, "dotenv", "pw")

	status, err := a.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Authenticated || !status.Unlocked {
		t.Fatalf("expected ready status, got %+v", status)
	}
	if status.Code != "" {
		t.Fatalf("expected no error code, got %q", status.Code)
	}
}

var _ provider.Provider = (*Adapter)(nil)
