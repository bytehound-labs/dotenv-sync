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

func TestPlanPushDocsMergesNoteJSONPayload(t *testing.T) {
	cfg := config.Config{
		ItemName:    "my-repo",
		StorageMode: config.StorageModeNoteJSON,
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("CURRENT=\nSCHEMA_ONLY=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("CURRENT=local-value\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "my-repo",
			Exists:   true,
			Env: map[string]string{
				"CURRENT":          "provider-value",
				"SCHEMA_ONLY":      "schema-value",
				"OTHER_BRANCH_KEY": "branch-value",
			},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if !plan.WriteRequired {
		t.Fatal("expected local update to require a write")
	}
	want := map[string]string{
		"CURRENT":          "local-value",
		"SCHEMA_ONLY":      "schema-value",
		"OTHER_BRANCH_KEY": "branch-value",
	}
	if !NoteJSONEqual(target.Env, want) {
		t.Fatal("merged payload did not apply local changes and retain provider keys")
	}
	if prov.payload.Env["CURRENT"] != "provider-value" {
		t.Fatal("planning mutated the loaded provider payload")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 1 || summary.Unchanged != 2 {
		t.Fatalf("unexpected push summary: %+v", summary)
	}
	for _, key := range []string{"SCHEMA_ONLY", "OTHER_BRANCH_KEY"} {
		found := false
		for _, change := range plan.Changes {
			if change.Key == key {
				found = true
				if change.ChangeType != "unchanged" {
					t.Fatalf("retained key %s change = %q; want unchanged", key, change.ChangeType)
				}
			}
		}
		if !found {
			t.Fatalf("missing preview for retained key %s", key)
		}
	}
}

func TestPlanPushDocsDoesNotWriteWhenEmptyLocalEnvMatchesNoteJSON(t *testing.T) {
	cfg := config.Config{
		ItemName:    "my-repo",
		StorageMode: config.StorageModeNoteJSON,
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("SCHEMA_ONLY=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, nil)
	providerEnv := map[string]string{
		"SCHEMA_ONLY":      "schema-value",
		"OTHER_BRANCH_KEY": "branch-value",
	}
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "my-repo",
			Exists:   true,
			Env:      providerEnv,
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if plan.WriteRequired {
		t.Fatal("an empty local env should not replace or rewrite an existing note payload")
	}
	if !NoteJSONEqual(target.Env, providerEnv) {
		t.Fatal("empty local env did not retain the existing provider payload")
	}
	if summary := Summarize(plan.Changes); summary.Unchanged != 2 {
		t.Fatalf("unexpected empty-env summary: %+v", summary)
	}
}

func TestPlanPushDocsExplicitBlankUpdatesNoteJSON(t *testing.T) {
	cfg := config.Config{
		ItemName:    "my-repo",
		StorageMode: config.StorageModeNoteJSON,
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("TOKEN=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("TOKEN=\n"))
	prov := &fakeProvider{
		payload: provider.EnvPayload{
			ItemName: "my-repo",
			Exists:   true,
			Env:      map[string]string{"TOKEN": "provider-value"},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if !plan.WriteRequired || target.Env["TOKEN"] != "" {
		t.Fatal("explicit blank should overwrite the provider value")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 1 {
		t.Fatalf("unexpected blank update summary: %+v", summary)
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
				"FILE":             "old-host-value",
				"JWT_SECRET":       "old-secret",
				"PORT":             "8080",
				"OTHER_BRANCH_KEY": "branch-value",
			},
		},
	}

	plan, target, err := PlanPushDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan push docs: %v", err)
	}
	if _, ok := target.Env["FILE"]; ok {
		t.Fatal("local key leaked into target payload")
	}
	if target.Env["JWT_SECRET"] != "new-secret" || target.Env["PORT"] != "8080" {
		t.Fatal("local values were not applied to the target payload")
	}
	if target.Env["OTHER_BRANCH_KEY"] != "branch-value" {
		t.Fatal("unrelated provider key was not retained")
	}
	if !plan.WriteRequired {
		t.Fatal("expected stale local provider value to require a write")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 2 || summary.Unchanged != 2 {
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
		t.Fatal("local key leaked into fields target")
	}
	if target.Env["JWT_SECRET"] != "secret" {
		t.Fatal("local provider value was not applied to the fields target")
	}
	if summary := Summarize(plan.Changes); summary.Updated != 1 {
		t.Fatalf("unexpected fields summary: %+v", summary)
	}
}
