package sync

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider"
)

func TestPlanPushDocsClassifiesChanges(t *testing.T) {
	cfg := config.Config{
		ItemName:    "my-repo",
		StorageMode: config.StorageModeNoteJSON,
		EnvFile:     filepath.Join(t.TempDir(), ".env"),
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("DATABASE_URL=postgres://vault/dev\nJWT_SECRET=supersecret\nPORT=8080\nEXTRA_FLAG=true\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "my-repo",
			Exists:   true,
			Env: map[string]string{
				"DATABASE_URL": "postgres://vault/old",
				"PORT":         "8080",
				"EXTRA_FLAG":   "false",
			},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if !plan.WriteRequired {
		t.Fatal("expected write required")
	}
	if summary := Summarize(plan.Changes); summary.Added != 1 || summary.Updated != 1 || summary.Unchanged != 1 || summary.Extra != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if target.ItemName != "my-repo" || !NoteJSONEqual(target.Env, CanonicalDocumentEnv(local)) {
		t.Fatalf("unexpected target payload: %+v", target)
	}
}

func TestPlanPushDocsSupportsFieldsModePasswordMapping(t *testing.T) {
	cfg := config.Config{
		ItemName:    "shared-dev",
		StorageMode: config.StorageModeFields,
		Mapping: map[string]string{
			"DB_PASSWD": "password",
		},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("DB_PASSWD=\nAPP_MODE=repo1\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("DB_PASSWD=rotated\nAPP_MODE=repo1\nEXTRA_FLAG=true\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName:    "shared-dev",
			StorageMode: config.StorageModeFields,
			Exists:      true,
			Password:    "old-secret",
			Notes:       "keep-me",
		},
		resolutions: map[string]provider.Resolution{
			"DB_PASSWD": {Key: "DB_PASSWD", Ref: "password", Source: "provider", Value: "old-secret"},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if !plan.WriteRequired {
		t.Fatal("expected write required")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 1 || summary.Extra != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if target.ItemName != "shared-dev" || target.StorageMode != config.StorageModeFields {
		t.Fatalf("unexpected target metadata: %+v", target)
	}
	if target.Notes != "keep-me" || target.Password != "old-secret" {
		t.Fatalf("expected existing raw item data to be preserved, got %+v", target)
	}
	if !NoteJSONEqual(target.Env, map[string]string{"DB_PASSWD": "rotated"}) {
		t.Fatalf("unexpected fields target env: %+v", target.Env)
	}
}

func TestPlanPushDocsRejectsUnsupportedFieldsModeMapping(t *testing.T) {
	cfg := config.Config{
		StorageMode: config.StorageModeFields,
		Mapping: map[string]string{
			"DB_PASSWD": "shared_password",
		},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("DB_PASSWD=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("DB_PASSWD=value\n"))

	if _, _, err := PlanPushDocs(context.Background(), cfg, schema, local, &fakeProvider{}); err == nil || !strings.Contains(err.Error(), "Bitwarden password field") {
		t.Fatalf("expected unsupported fields-mode mapping error, got %v", err)
	}
}

func TestPlanPushDocsExcludesLocalKeysAndRemovesStaleNoteJSONValues(t *testing.T) {
	cfg := config.Config{
		ItemName:    "my-repo",
		StorageMode: config.StorageModeNoteJSON,
		LocalKeys:   []string{"FILE"},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=\nJWT_SECRET=\nPORT=8080\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("FILE=1\nJWT_SECRET=new-secret\nPORT=8080\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "my-repo",
			Exists:   true,
			Env: map[string]string{
				"FILE":       "old-host-value",
				"JWT_SECRET": "old-secret",
				"PORT":       "8080",
			},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if _, ok := target.Env["FILE"]; ok {
		t.Fatalf("local key leaked into target payload: %#v", target.Env)
	}
	if target.Env["JWT_SECRET"] != "new-secret" || target.Env["PORT"] != "8080" {
		t.Fatalf("unexpected target payload: %#v", target.Env)
	}
	if !plan.WriteRequired {
		t.Fatal("expected stale local provider value to require a write")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 2 || summary.Unchanged != 1 {
		t.Fatalf("unexpected push summary: %+v", summary)
	}
	for _, change := range plan.Changes {
		if change.Key == "FILE" && strings.Contains(change.After, "1") {
			t.Fatalf("local value leaked in change: %+v", change)
		}
	}
}

func TestPlanPushDocsExcludesLocalKeysFromFieldsMode(t *testing.T) {
	cfg := config.Config{
		ItemName:    "shared-dev",
		StorageMode: config.StorageModeFields,
		LocalKeys:   []string{"FILE"},
		Mapping:     map[string]string{"JWT_SECRET": "password"},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=\nJWT_SECRET=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("FILE=1\nJWT_SECRET=secret\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "shared-dev",
			Exists:   true,
			Password: "old-secret",
		},
		resolutions: map[string]provider.Resolution{
			"JWT_SECRET": {Key: "JWT_SECRET", Ref: "password", Source: "provider", Value: "old-secret"},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if _, ok := target.Env["FILE"]; ok {
		t.Fatalf("local key leaked into fields target: %#v", target.Env)
	}
	if target.Env["JWT_SECRET"] != "secret" {
		t.Fatalf("unexpected fields target: %#v", target.Env)
	}
	if summary := Summarize(plan.Changes); summary.Updated != 1 {
		t.Fatalf("unexpected fields summary: %+v", summary)
	}
}
