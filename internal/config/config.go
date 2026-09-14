package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	StorageModeFields   = "fields"
	StorageModeNoteJSON = "note_json"
)

type Config struct {
	Provider    string            `yaml:"provider"`
	SchemaFile  string            `yaml:"schema_file"`
	EnvFile     string            `yaml:"env_file"`
	ItemName    string            `yaml:"item_name"`
	StorageMode string            `yaml:"storage_mode"`
	Vault       string            `yaml:"vault"`
	Mapping     map[string]string `yaml:"mapping"`
	LocalKeys   []string          `yaml:"local_keys"`
	ConfigFile  string            `yaml:"-"`
	BaseDir     string            `yaml:"-"`

	// KeePass-specific fields. Ignored when provider is "bitwarden".
	KeePassDatabase string `yaml:"keepass_database"` // path to the .kdbx file
	KeePassGroup    string `yaml:"keepass_group"`    // group inside the vault that holds env vars

	localKeySet map[string]struct{}
}

type LoadOptions struct {
	ConfigPath string
	SchemaPath string
	EnvPath    string
}

func Default(baseDir string) Config {
	return Config{
		Provider:    "bitwarden",
		SchemaFile:  filepath.Join(baseDir, ".env.example"),
		EnvFile:     filepath.Join(baseDir, ".env"),
		ItemName:    defaultItemName(baseDir),
		StorageMode: StorageModeFields,
		ConfigFile:  filepath.Join(baseDir, ".envsync.yaml"),
		Mapping:     map[string]string{},
		LocalKeys:   []string{},
		BaseDir:     baseDir,
	}
}

func Load(baseDir string, opts LoadOptions) (Config, error) {
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return Config{}, err
	}
	cfg := Default(baseDir)
	if opts.ConfigPath != "" {
		cfg.ConfigFile = resolvePath(baseDir, opts.ConfigPath)
	}
	if data, err := os.ReadFile(cfg.ConfigFile); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, err
	}
	cfg.BaseDir = baseDir
	cfg.ItemName = strings.TrimSpace(cfg.ItemName)
	if cfg.ItemName == "" {
		cfg.ItemName = defaultItemName(baseDir)
	}
	cfg.StorageMode = normalizeStorageMode(cfg.StorageMode)
	if cfg.StorageMode == "" {
		cfg.StorageMode = StorageModeFields
	}
	cfg.ConfigFile = resolvePath(baseDir, cfg.ConfigFile)
	cfg.SchemaFile = resolvePath(baseDir, cfg.SchemaFile)
	cfg.EnvFile = resolvePath(baseDir, cfg.EnvFile)
	if opts.SchemaPath != "" {
		cfg.SchemaFile = resolvePath(baseDir, opts.SchemaPath)
	}
	if opts.EnvPath != "" {
		cfg.EnvFile = resolvePath(baseDir, opts.EnvPath)
	}
	if cfg.Provider == "" {
		cfg.Provider = "bitwarden"
	}
	if cfg.Mapping == nil {
		cfg.Mapping = map[string]string{}
	}
	localKeys, localKeySet, err := normalizeLocalKeys(cfg.LocalKeys)
	if err != nil {
		return Config{}, err
	}
	cfg.LocalKeys = localKeys
	cfg.localKeySet = localKeySet
	for key := range localKeySet {
		if _, ok := cfg.Mapping[key]; ok {
			return Config{}, fmt.Errorf("local key %q must not also appear in mapping", key)
		}
	}
	// Resolve the KeePass database path relative to the base directory,
	// the same way schema_file and env_file are resolved.
	if cfg.KeePassDatabase != "" {
		cfg.KeePassDatabase = resolvePath(baseDir, cfg.KeePassDatabase)
	}
	// Default the KeePass group to "dotenv" if not specified.
	if cfg.KeePassGroup == "" {
		cfg.KeePassGroup = "dotenv"
	}
	if cfg.StorageMode != StorageModeFields && cfg.StorageMode != StorageModeNoteJSON {
		return Config{}, fmt.Errorf("storage_mode must be %q or %q", StorageModeFields, StorageModeNoteJSON)
	}
	if filepath.Clean(cfg.SchemaFile) == filepath.Clean(cfg.EnvFile) {
		return Config{}, fmt.Errorf("schema_file and env_file must differ")
	}
	return cfg, nil
}

func normalizeStorageMode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (c Config) UsesNoteJSON() bool {
	return normalizeStorageMode(c.StorageMode) == StorageModeNoteJSON
}

func (c Config) ProviderRef(key string) string {
	if c.UsesNoteJSON() {
		return key
	}
	if ref := strings.TrimSpace(c.Mapping[key]); ref != "" {
		return ref
	}
	return key
}

func (c Config) IsLocalKey(key string) bool {
	key = strings.TrimSpace(key)
	if c.localKeySet != nil {
		_, ok := c.localKeySet[key]
		return ok
	}
	for _, localKey := range c.LocalKeys {
		if strings.TrimSpace(localKey) == key {
			return true
		}
	}
	return false
}

func normalizeLocalKeys(values []string) ([]string, map[string]struct{}, error) {
	normalized := make([]string, 0, len(values))
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			return nil, nil, fmt.Errorf("local_keys entries must not be blank")
		}
		if _, exists := set[key]; exists {
			return nil, nil, fmt.Errorf("local_keys contains duplicate key %q", key)
		}
		normalized = append(normalized, key)
		set[key] = struct{}{}
	}
	return normalized, set, nil
}

func resolvePath(baseDir, value string) string {
	if value == "" {
		return value
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if strings.HasPrefix(value, baseDir+string(filepath.Separator)) {
		return filepath.Clean(value)
	}
	return filepath.Join(baseDir, value)
}

func defaultItemName(baseDir string) string {
	root := repoRoot(baseDir)
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = filepath.Base(baseDir)
	}
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "dotenv-sync"
	}
	return name
}

func repoRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(start)
		}
		dir = parent
	}
}
