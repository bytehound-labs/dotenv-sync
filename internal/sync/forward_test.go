package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/envfile"
	"dotenv-sync/internal/provider"
	"dotenv-sync/internal/report"
)

type captureResolveProvider struct {
	refs map[string]string
}

func (c *captureResolveProvider) Name() string { return "capture" }

func (c *captureResolveProvider) CheckReadiness(context.Context) (provider.Status, error) {
	return provider.Status{Provider: "bitwarden", CLIInstalled: true, Authenticated: true, Unlocked: true}, nil
}

func (c *captureResolveProvider) Resolve(context.Context, string, string) (provider.Resolution, error) {
	return provider.Resolution{}, nil
}

func (c *captureResolveProvider) ResolveMany(_ context.Context, refs map[string]string) (map[string]provider.Resolution, error) {
	c.refs = refs
	return map[string]provider.Resolution{
		"DATABASE_URL": {Source: "provider", Value: "postgres://vault/dev"},
	}, nil
}

type countingProvider struct {
	readyCalls   int
	resolveCalls int
	refs         map[string]string
	results      map[string]provider.Resolution
}

func (c *countingProvider) Name() string { return "counting" }

func (c *countingProvider) CheckReadiness(context.Context) (provider.Status, error) {
	c.readyCalls++
	return provider.Status{Provider: "bitwarden", CLIInstalled: true, Authenticated: true, Unlocked: true}, nil
}

func (c *countingProvider) Resolve(context.Context, string, string) (provider.Resolution, error) {
	return provider.Resolution{}, nil
}

func (c *countingProvider) ResolveMany(_ context.Context, refs map[string]string) (map[string]provider.Resolution, error) {
	c.resolveCalls++
	c.refs = refs
	return c.results, nil
}

func TestPlanForwardDocsClassifiesChanges(t *testing.T) {
	cfg := config.Config{EnvFile: filepath.Join(t.TempDir(), ".env"), Mapping: map[string]string{"DATABASE_URL": "database-url", "JWT_SECRET": "jwt-secret"}}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("DATABASE_URL=\nJWT_SECRET=\nPORT=8080\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("DATABASE_URL=postgres://vault/dev\nPORT=9090\n"))
	prov := fakeProvider{resolutions: map[string]provider.Resolution{
		"DATABASE_URL": {Source: "provider", Value: "postgres://vault/dev"},
		"JWT_SECRET":   {Source: "provider", Value: "topsecret"},
	}}

	plan, target, err := PlanForwardDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan forward docs returned error: %v", err)
	}
	if !plan.WriteRequired {
		t.Fatalf("expected write required")
	}
	got := string(envfile.Render(target))
	for _, want := range []string{"DATABASE_URL=postgres://vault/dev", "JWT_SECRET=topsecret", "PORT=8080"} {
		if !strings.Contains(got, want) {
			t.Fatalf("target output missing %q\n%s", want, got)
		}
	}

	summary := Summarize(plan.Changes)
	if summary.Added != 1 || summary.Updated != 1 || summary.Unchanged != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestPlanForwardDocsIgnoresFieldMappingsInNoteJSONMode(t *testing.T) {
	cfg := config.Config{
		EnvFile:     filepath.Join(t.TempDir(), ".env"),
		StorageMode: config.StorageModeNoteJSON,
		Mapping:     map[string]string{"DATABASE_URL": "database-url"},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("DATABASE_URL=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("DATABASE_URL=postgres://vault/dev\n"))
	prov := &captureResolveProvider{}

	if _, _, err := PlanForwardDocs(context.Background(), cfg, schema, local, prov); err != nil {
		t.Fatalf("plan forward docs: %v", err)
	}
	if got := prov.refs["DATABASE_URL"]; got != "DATABASE_URL" {
		t.Fatalf("expected note_json ref to stay on env key, got %q", got)
	}
}

func TestPlanForwardDocsPreservesDifferentLocalValuesWithoutProviderCalls(t *testing.T) {
	cfg := config.Config{
		LocalKeys: []string{"FILE"},
		EnvFile:   filepath.Join(t.TempDir(), ".env"),
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=\nPORT=8080\n"))
	prov := &countingProvider{}

	for _, hostValue := range []string{"1", "2"} {
		local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("FILE="+hostValue+"\nPORT=8080\n"))
		plan, target, err := PlanForwardDocs(context.Background(), cfg, schema, local, prov)
		if err != nil {
			t.Fatalf("plan host %s: %v", hostValue, err)
		}
		if len(plan.Warnings) != 0 {
			t.Fatalf("unexpected warnings for host %s: %#v", hostValue, plan.Warnings)
		}
		if got := target.AssignmentMap()["FILE"].Value; got != hostValue {
			t.Fatalf("host %s target FILE=%q", hostValue, got)
		}
	}
	if prov.readyCalls != 0 || prov.resolveCalls != 0 {
		t.Fatalf("local/static schema should not call provider: ready=%d resolve=%d", prov.readyCalls, prov.resolveCalls)
	}
}

func TestPlanForwardDocsExcludesLocalKeysFromProviderResolution(t *testing.T) {
	cfg := config.Config{
		LocalKeys: []string{"FILE"},
		EnvFile:   filepath.Join(t.TempDir(), ".env"),
		Mapping:   map[string]string{"DATABASE_URL": "database-url"},
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=\nDATABASE_URL=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte("FILE=1\nDATABASE_URL=old\n"))
	prov := &countingProvider{
		results: map[string]provider.Resolution{
			"DATABASE_URL": {Source: "provider", Value: "postgres://vault/dev"},
		},
	}

	plan, target, err := PlanForwardDocs(context.Background(), cfg, schema, local, prov)
	if err != nil {
		t.Fatalf("plan forward docs: %v", err)
	}
	if prov.readyCalls != 1 || prov.resolveCalls != 1 {
		t.Fatalf("expected one provider call, got ready=%d resolve=%d", prov.readyCalls, prov.resolveCalls)
	}
	if len(prov.refs) != 1 || prov.refs["DATABASE_URL"] != "database-url" {
		t.Fatalf("unexpected provider refs: %#v", prov.refs)
	}
	if got := target.AssignmentMap()["FILE"].Value; got != "1" {
		t.Fatalf("expected local FILE to survive, got %q", got)
	}
	if got := target.AssignmentMap()["DATABASE_URL"].Value; got != "postgres://vault/dev" {
		t.Fatalf("expected provider value, got %q", got)
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", plan.Warnings)
	}
}

func TestPlanForwardDocsWarnsWithoutBlockingMissingLocalValue(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		LocalKeys:  []string{"FILE"},
		SchemaFile: filepath.Join(dir, ".env.example"),
		EnvFile:    filepath.Join(dir, ".env"),
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=\n"))
	local := envfile.ParseBytes(".env", envfile.KindLocal, []byte(""))
	if err := os.WriteFile(cfg.SchemaFile, []byte("FILE=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.EnvFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, target, err := PlanForwardDocs(context.Background(), cfg, schema, local, &countingProvider{})
	if err != nil {
		t.Fatalf("missing local value should be warning-only: %v", err)
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0].Key != "FILE" {
		t.Fatalf("expected one local warning, got %#v", plan.Warnings)
	}
	if got := target.AssignmentMap()["FILE"].Value; got != "" {
		t.Fatalf("expected blank local target, got %q", got)
	}
	if plan.WriteRequired != true {
		t.Fatal("expected missing local assignment to require a write")
	}
	if got := SummarizePlan(plan).Warnings; got != 1 {
		t.Fatalf("expected warning summary count 1, got %d", got)
	}

	validatePlan, err := PlanValidate(context.Background(), cfg, &countingProvider{})
	if err != nil {
		t.Fatalf("local warning should not fail validate: %v", err)
	}
	if len(validatePlan.Warnings) != 1 {
		t.Fatalf("expected validate warning, got %#v", validatePlan.Warnings)
	}
}

func TestPlanForwardDocsRejectsInvalidLocalSchema(t *testing.T) {
	cfg := config.Config{
		LocalKeys: []string{"FILE"},
		EnvFile:   filepath.Join(t.TempDir(), ".env"),
	}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=1\n"))
	prov := &countingProvider{}

	plan, _, err := PlanForwardDocs(context.Background(), cfg, schema, envfile.Document{}, prov)
	if report.ExitCode(err) != report.ExitValidation {
		t.Fatalf("expected validation exit, got %v", err)
	}
	if prov.readyCalls != 0 || prov.resolveCalls != 0 {
		t.Fatalf("invalid local schema should not call provider: ready=%d resolve=%d", prov.readyCalls, prov.resolveCalls)
	}
	if len(plan.Issues) != 1 || plan.Issues[0].Code != "E012" {
		t.Fatalf("unexpected local schema issues: %#v", plan.Issues)
	}
}
