package keepass

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"dotenv-sync/internal/config"
	"dotenv-sync/internal/provider"

	"golang.org/x/term"
)

// Adapter implements provider.Provider using KeePassXC as the secret backend.
// It satisfies the same interface as the Bitwarden adapter, so the rest of the
// tool (sync, diff, validate, doctor) works without any changes.
type Adapter struct {
	client *KPXCClient
	cfg    config.Config

	// mu guards the resolution cache so concurrent ResolveMany calls are safe.
	mu    sync.Mutex
	cache map[string]provider.Resolution
}

// NewAdapter creates a KeePass adapter from the loaded config.
// The master password is NOT prompted here — that happens lazily on the first
// call that actually needs the database, so `ds doctor` can still check
// whether the binary is installed without asking for a password.
func NewAdapter(cfg config.Config) *Adapter {
	return &Adapter{
		client: NewKPXCClient(),
		cfg:    cfg,
		cache:  map[string]provider.Resolution{},
	}
}

// Name identifies this provider in output and config.
func (a *Adapter) Name() string { return "keepass" }

// CheckReadiness verifies that keepassxc-cli is installed and that the
// configured database file exists and is openable with the current password.
// This is what `ds doctor` calls.
func (a *Adapter) CheckReadiness(ctx context.Context) (provider.Status, error) {
	status := provider.Status{Provider: "keepass", CLIInstalled: true}

	// Check 1: is the binary on PATH?
	if _, err := exec.LookPath(a.client.Bin); err != nil {
		status.CLIInstalled = false
		status.Code = "E001"
		status.Problem = "keepassxc-cli not installed"
		status.Impact = "commands cannot reach KeePass database"
		status.Action = "install keepassxc-cli or add it to PATH"
		return status, nil
	}

	// Check 2: does the database file exist?
	if _, err := os.Stat(a.cfg.KeePassDatabase); err != nil {
		status.Code = "E002"
		status.Problem = fmt.Sprintf("KeePass database not found: %s", a.cfg.KeePassDatabase)
		status.Impact = "commands cannot open the KeePass database"
		status.Action = "check keepass_database path in .envsync.yaml"
		return status, nil
	}

	// Check 3: can we unlock the database?
	// This is the step that prompts for the password if not already set.
	if err := a.ensurePassword(ctx); err != nil {
		status.Code = "E003"
		status.Problem = "KeePass database could not be unlocked"
		status.Impact = "commands cannot read secrets from KeePass"
		status.Action = "check your master password and retry"
		return status, nil
	}

	// Check 4: can we reach the configured group?
	_, err := a.client.ListGroup(ctx, a.cfg.KeePassDatabase, a.cfg.KeePassGroup)
	if err != nil {
		status.Code = "E004"
		status.Problem = fmt.Sprintf("KeePass group %q not found or not readable", a.cfg.KeePassGroup)
		status.Impact = "sync cannot resolve provider-managed schema keys"
		status.Action = "check keepass_group in .envsync.yaml"
		return status, nil
	}

	status.Authenticated = true
	status.Unlocked = true
	status.Message = "keepassxc-cli is ready"
	return status, nil
}

// Resolve fetches the secret value for a single env key from KeePass.
// The entry is looked up at <group>/<key> inside the database.
// Results are cached so the same key is only fetched once per run.
func (a *Adapter) Resolve(ctx context.Context, key, _ string) (provider.Resolution, error) {
	// The second argument (ref) is the provider-specific field name used by
	// Bitwarden's field mapping. KeePass uses the entry path directly, so we
	// ignore it and derive the path from the group + key name.
	cacheKey := a.cfg.KeePassGroup + "/" + key

	a.mu.Lock()
	if cached, ok := a.cache[cacheKey]; ok {
		a.mu.Unlock()
		return cached, nil
	}
	a.mu.Unlock()

	// Prompt for the password on the first real resolution if not yet set.
	if err := a.ensurePassword(ctx); err != nil {
		return provider.Resolution{}, err
	}

	entryPath := cacheKey // e.g. "dotenv/DATABASE_URL"
	value, err := a.client.Show(ctx, a.cfg.KeePassDatabase, entryPath)

	resolution := provider.Resolution{Key: key, Ref: entryPath}
	if err != nil {
		if err == ErrItemNotFound {
			resolution.Source = "missing"
			resolution.IssueCode = "E005"
		} else {
			resolution.Source = "error"
			resolution.IssueCode = "E003"
		}
	} else {
		resolution.Source = "provider"
		resolution.Value = value
	}

	a.mu.Lock()
	a.cache[cacheKey] = resolution
	a.mu.Unlock()

	return resolution, nil
}

// ResolveMany fetches secrets for multiple keys in one go.
// Each key is resolved individually (keepassxc-cli has no batch mode),
// but results are cached so the password is only prompted once.
func (a *Adapter) ResolveMany(ctx context.Context, refs map[string]string) (map[string]provider.Resolution, error) {
	results := make(map[string]provider.Resolution, len(refs))
	for key, ref := range refs {
		res, err := a.Resolve(ctx, key, ref)
		if err != nil {
			return nil, err
		}
		results[key] = res
	}
	return results, nil
}

// SetPassword pre-seeds the master password without prompting.
// Used by ds scaffold when the password was already collected during first-run setup.
func (a *Adapter) SetPassword(password string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.client.Password = password
}

// EnsurePassword is the public form of ensurePassword, used by ds scaffold
// which needs to prompt for the password before making multiple CLI calls.
func (a *Adapter) EnsurePassword(ctx context.Context) error {
	return a.ensurePassword(ctx)
}

// CreateEntry creates a blank entry in the KeePass group for the given key.
// Returns ErrEntryExists if the entry already exists — callers should skip
// rather than treat this as an error.
func (a *Adapter) CreateEntry(ctx context.Context, key string) error {
	if err := a.ensurePassword(ctx); err != nil {
		return err
	}
	return a.client.CreateEntry(ctx, a.cfg.KeePassDatabase, a.cfg.KeePassGroup, key, "")
}

// ensurePassword prompts the user for the KeePass master password exactly once
// per process run. Subsequent calls return immediately because the password is
// already stored in the client.
//
// The prompt writes to stderr so it does not pollute stdout (which may be
// parsed by scripts). The password is read without echo using golang.org/x/term.
func (a *Adapter) ensurePassword(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Already have the password from a previous call — nothing to do.
	if a.client.Password != "" {
		return nil
	}

	// Print the prompt to stderr so stdout stays clean.
	fmt.Fprintf(os.Stderr, "Enter KeePass master password for %s: ", a.cfg.KeePassDatabase)

	// term.ReadPassword reads a line without echoing it to the terminal.
	// It takes the file descriptor of stdin (0).
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after the hidden input
	if err != nil {
		return fmt.Errorf("could not read master password: %w", err)
	}

	a.client.Password = strings.TrimSpace(string(pw))
	return nil
}
