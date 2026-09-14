package bitwarden

import (
	"context"
	"strings"
	"testing"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/testutil"
)

func TestAdapterResolveUsesRepoItemAndFieldOverride(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Fields: map[string]string{
			"my-repo::db_url": "postgres://vault/dev",
		},
	})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg:    config.Config{ItemName: "my-repo"},
		cache:  map[string]provider.Resolution{},
	}

	resolution, err := adapter.Resolve(context.Background(), "DATABASE_URL", "db_url")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Source != "provider" || resolution.Value != "postgres://vault/dev" {
		t.Fatalf("unexpected resolution: %+v", resolution)
	}

	if _, err := adapter.Resolve(context.Background(), "DATABASE_URL", "db_url"); err != nil {
		t.Fatalf("resolve from cache: %v", err)
	}
	if got := strings.Count(stub.Log(t), "get --field db_url my-repo"); got != 1 {
		t.Fatalf("expected one rbw invocation, got %d log=%q", got, stub.Log(t))
	}
}

func TestAdapterResolveManyUsesNoteJSONPayloadOnce(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Items: map[string]testutil.RBWStubItem{
			"my-repo": {
				Notes:    `{"format":"dotenv-sync/note-json@v1","env":{"DATABASE_URL":"postgres://vault/dev","JWT_SECRET":"supersecret"}}`,
				Password: "keep-me",
			},
		},
	})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg:    config.Config{ItemName: "my-repo", StorageMode: config.StorageModeNoteJSON},
		cache:  map[string]provider.Resolution{},
	}

	results, err := adapter.ResolveMany(context.Background(), map[string]string{
		"DATABASE_URL": "ignored-mapping",
		"JWT_SECRET":   "also-ignored",
		"MISSING_KEY":  "still-ignored",
	})
	if err != nil {
		t.Fatalf("resolve many: %v", err)
	}
	if results["DATABASE_URL"].Value != "postgres://vault/dev" || results["JWT_SECRET"].Value != "supersecret" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results["MISSING_KEY"].IssueCode != "E005" {
		t.Fatalf("expected missing note_json key to use E005, got %+v", results["MISSING_KEY"])
	}
	if _, err := adapter.Resolve(context.Background(), "DATABASE_URL", "ignored"); err != nil {
		t.Fatalf("resolve from cached payload: %v", err)
	}
	if got := strings.Count(stub.Log(t), "get --raw my-repo"); got != 1 {
		t.Fatalf("expected one raw rbw invocation, got %d log=%q", got, stub.Log(t))
	}
}

func TestAdapterLoadEnvPayloadRejectsMalformedNotes(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Items: map[string]testutil.RBWStubItem{
			"my-repo": {Notes: "not-json", Password: "keep-me"},
		},
	})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg:    config.Config{ItemName: "my-repo", StorageMode: config.StorageModeNoteJSON},
		cache:  map[string]provider.Resolution{},
	}

	if _, err := adapter.LoadEnvPayload(context.Background()); err == nil || !strings.Contains(err.Error(), "provider note_json payload is malformed") {
		t.Fatalf("expected malformed payload error, got %v", err)
	}
}

func TestAdapterLoadEnvPayloadSupportsFieldsMode(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Items: map[string]testutil.RBWStubItem{
			"shared-dev": {Notes: "keep-me", Password: "shared-secret"},
		},
	})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg:    config.Config{ItemName: "shared-dev", StorageMode: config.StorageModeFields},
	}

	payload, err := adapter.LoadEnvPayload(context.Background())
	if err != nil {
		t.Fatalf("load fields payload: %v", err)
	}
	if !payload.Exists || payload.Password != "shared-secret" || payload.Notes != "keep-me" {
		t.Fatalf("unexpected fields payload: %+v", payload)
	}
}

func TestAdapterStoreEnvPayloadSupportsFieldsModePasswordWrites(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Items: map[string]testutil.RBWStubItem{
			"shared-dev": {Notes: "keep-me", Password: "old-secret"},
		},
	})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg: config.Config{
			ItemName:    "shared-dev",
			StorageMode: config.StorageModeFields,
			Mapping: map[string]string{
				"DB_PASSWD": "password",
			},
		},
	}

	_, err := adapter.StoreEnvPayload(context.Background(), provider.EnvPayload{
		ItemName:    "shared-dev",
		StorageMode: config.StorageModeFields,
		Exists:      true,
		Notes:       "keep-me",
		Password:    "old-secret",
		Env: map[string]string{
			"DB_PASSWD": "rotated-secret",
		},
	})
	if err != nil {
		t.Fatalf("store fields payload: %v", err)
	}
	if got := stub.Password(t, "shared-dev"); got != "rotated-secret" {
		t.Fatalf("unexpected password after update: %q", got)
	}
	if got := stub.Note(t, "shared-dev"); got != "keep-me" {
		t.Fatalf("unexpected notes after update: %q", got)
	}
	logData := stub.Log(t)
	if !strings.Contains(logData, "edit shared-dev") || !strings.Contains(logData, "sync") {
		t.Fatalf("unexpected rbw log: %s", logData)
	}
}

func TestAdapterResolveTreatsNoEntryFoundAsMissing(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{Status: "unlocked"})
	stub.SetEnv(t)
	adapter := &Adapter{
		client: &RBWClient{Bin: stub.Path()},
		cfg:    config.Config{ItemName: "my-repo"},
		cache:  map[string]provider.Resolution{},
	}

	resolution, err := adapter.Resolve(context.Background(), "DATABASE_URL", "DATABASE_URL")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Source != "missing" || resolution.IssueCode != "E005" {
		t.Fatalf("expected missing/E005, got %+v", resolution)
	}
}
