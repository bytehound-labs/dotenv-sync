package config

import (
	"testing"

	"dotenv-sync/internal/envfile"
)

func TestClassifySourcePrioritizesLocalKeys(t *testing.T) {
	cfg := Config{LocalKeys: []string{"FILE"}}

	if got := cfg.ClassifySource("FILE", true); got != SourceLocal {
		t.Fatalf("expected local source, got %q", got)
	}
	if got := cfg.ClassifySource("SECRET", true); got != SourceProvider {
		t.Fatalf("expected provider source, got %q", got)
	}
	if got := cfg.ClassifySource("PORT", false); got != SourceStatic {
		t.Fatalf("expected static source, got %q", got)
	}
}

func TestValidateLocalKeysRequiresBlankUniqueSchemaEntries(t *testing.T) {
	cfg := Config{LocalKeys: []string{"FILE", "MISSING"}}
	schema := envfile.ParseBytes(".env.example", envfile.KindSchema, []byte("FILE=1\nFILE=2\n"))

	issues := cfg.ValidateLocalKeys(schema)
	if len(issues) != 2 {
		t.Fatalf("expected two local schema issues, got %#v", issues)
	}
	if issues[0].Key != "FILE" || issues[1].Key != "MISSING" {
		t.Fatalf("unexpected issue keys: %#v", issues)
	}
}
