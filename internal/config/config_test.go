package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsItemNameFromRepoRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(repo, "services", "api")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(project, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ItemName != filepath.Base(repo) {
		t.Fatalf("expected item name %q, got %q", filepath.Base(repo), cfg.ItemName)
	}
	if cfg.SchemaFile != filepath.Join(project, ".env.example") {
		t.Fatalf("unexpected schema file: %s", cfg.SchemaFile)
	}
	if cfg.StorageMode != StorageModeFields {
		t.Fatalf("expected default storage mode %q, got %q", StorageModeFields, cfg.StorageMode)
	}
}

func TestLoadDefaultsItemNameWithoutGit(t *testing.T) {
	project := t.TempDir()

	cfg, err := Load(project, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ItemName != filepath.Base(project) {
		t.Fatalf("expected fallback item name %q, got %q", filepath.Base(project), cfg.ItemName)
	}
}

func TestLoadItemNameOverrideFromConfig(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".envsync.yaml"), []byte("item_name: shared-dev\nstorage_mode: note_json\nmapping:\n  DATABASE_URL: db_url\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(project, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ItemName != "shared-dev" {
		t.Fatalf("expected override item name, got %q", cfg.ItemName)
	}
	if got := cfg.Mapping["DATABASE_URL"]; got != "db_url" {
		t.Fatalf("expected field override db_url, got %q", got)
	}
	if cfg.StorageMode != StorageModeNoteJSON {
		t.Fatalf("expected note_json storage mode, got %q", cfg.StorageMode)
	}
}

func TestLoadRejectsInvalidStorageMode(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".envsync.yaml"), []byte("storage_mode: unsupported\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(project, LoadOptions{}); err == nil {
		t.Fatal("expected invalid storage mode error")
	}
}

func TestLoadNormalizesLocalKeys(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".envsync.yaml"), []byte("local_keys:\n  - ' FILE '\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(project, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.LocalKeys) != 1 || cfg.LocalKeys[0] != "FILE" {
		t.Fatalf("expected normalized local keys, got %#v", cfg.LocalKeys)
	}
	if !cfg.IsLocalKey("FILE") || !cfg.IsLocalKey(" FILE ") {
		t.Fatal("expected IsLocalKey to normalize lookups")
	}
	if cfg.IsLocalKey("OTHER") {
		t.Fatal("unexpected local key match")
	}
}

func TestLoadRejectsInvalidLocalKeys(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "blank", yaml: "local_keys:\n  - ' '\n"},
		{name: "duplicate", yaml: "local_keys:\n  - FILE\n  - ' FILE '\n"},
		{name: "provider mapping", yaml: "local_keys:\n  - FILE\nmapping:\n  FILE: file\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, ".envsync.yaml"), []byte(test.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(project, LoadOptions{}); err == nil {
				t.Fatal("expected invalid local_keys error")
			}
		})
	}
}
