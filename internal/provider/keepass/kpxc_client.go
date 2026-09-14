package keepass

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrBinaryMissing is returned when keepassxc-cli is not found on PATH.
var ErrBinaryMissing = errors.New("keepassxc-cli binary missing")

// ErrItemNotFound is returned when the requested entry does not exist in the database.
var ErrItemNotFound = errors.New("keepassxc-cli entry not found")

// ErrEntryExists is returned when trying to create an entry that already exists.
var ErrEntryExists = errors.New("keepassxc-cli entry already exists")

// KPXCClient wraps the keepassxc-cli binary.
// It holds the path to the binary and the master password for the database.
// The password is prompted once by the Adapter and stored here for the
// lifetime of a single ds sync run — it is never written to disk.
type KPXCClient struct {
	// Bin is the name or path of the keepassxc-cli binary.
	// Defaults to "keepassxc-cli".
	Bin string

	// Password is the master password for the KeePass database.
	// Supplied once at startup and reused for every CLI call.
	Password string
}

// NewKPXCClient returns a client using the default binary name.
// Password is set later by the Adapter after prompting the user.
func NewKPXCClient() *KPXCClient {
	return &KPXCClient{Bin: "keepassxc-cli"}
}

// Show retrieves a single entry from the database and returns the value of
// its Password field.
//
// entryPath is the full path to the entry inside the database, e.g.
// "dotenv/DATABASE_URL".  databasePath is the path to the .kdbx file.
//
// Equivalent shell command:
//
//	echo "<password>" | keepassxc-cli show -s <database> <entryPath>
func (c *KPXCClient) Show(ctx context.Context, databasePath, entryPath string) (string, error) {
	out, err := c.run(ctx, databasePath, "show", "-s", databasePath, entryPath)
	if err != nil {
		return "", err
	}
	return parsePasswordField(out)
}

// ListGroup returns the entry titles inside a group, confirming the group
// exists and is reachable.
//
// Equivalent shell command:
//
//	echo "<password>" | keepassxc-cli ls <database> <group>
func (c *KPXCClient) ListGroup(ctx context.Context, databasePath, group string) ([]string, error) {
	out, err := c.run(ctx, databasePath, "ls", databasePath, group)
	if err != nil {
		return nil, err
	}
	// Each line is an entry title or sub-group (sub-groups end with "/").
	var entries []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasSuffix(line, "/") {
			entries = append(entries, line)
		}
	}
	return entries, nil
}

// run executes keepassxc-cli, piping the master password via stdin.
// args are the CLI arguments that follow the binary name.
// databasePath is separated out so we can check for not-found errors.
func (c *KPXCClient) run(ctx context.Context, databasePath string, args ...string) (string, error) {
	// Make sure the binary exists on PATH before trying to run it.
	if _, err := exec.LookPath(c.Bin); err != nil {
		return "", ErrBinaryMissing
	}

	cmd := exec.CommandContext(ctx, c.Bin, args...)

	// keepassxc-cli reads the master password from stdin when it is not a TTY.
	// We write "<password>\n" to stdin so the prompt is answered automatically.
	cmd.Stdin = strings.NewReader(c.Password + "\n")

	out, err := cmd.CombinedOutput()
	// CombinedOutput captures both stdout and stderr together.
	text := strings.TrimSpace(string(out))

	if err != nil {
		if isNotFoundText(text, err) {
			return "", ErrItemNotFound
		}
		if text == "" {
			return "", fmt.Errorf("keepassxc-cli %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}

// CreateEntry creates a new entry in the database under the given group.
// The entry title is set to entryName and the password field is left blank —
// the user fills in the real value in KeePassXC after scaffolding.
//
// Returns ErrEntryExists if the entry already exists — callers should skip
// rather than treat this as an error.
//
// Equivalent shell command:
//
//	echo "<password>" | keepassxc-cli add -p <database> <group>/<entryName>
func (c *KPXCClient) CreateEntry(ctx context.Context, databasePath, group, entryName, entryPassword string) error {
	if _, err := exec.LookPath(c.Bin); err != nil {
		return ErrBinaryMissing
	}

	// keepassxc-cli gives the same "Could not create entry" message for both
	// "already exists" and genuine failures, so we can't distinguish from the
	// output alone. Instead, check existence first with a ListGroup call.
	entries, err := c.ListGroup(ctx, databasePath, group)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e == entryName {
			return ErrEntryExists
		}
	}

	entryPath := group + "/" + entryName
	// keepassxc-cli add -p reads two lines from stdin:
	//   line 1: database master password
	//   line 2: new entry password (prompted by -p flag)
	input := c.Password + "\n" + entryPassword + "\n"

	cmd := exec.CommandContext(ctx, c.Bin, "add", "-p", databasePath, entryPath)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return fmt.Errorf("keepassxc-cli add failed: %w", err)
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

// parsePasswordField scans the output of `keepassxc-cli show -s` and
// extracts the value after "Password: ".
//
// Example output:
//
//	Title: DATABASE_URL
//	UserName:
//	Password: postgres://localhost:5432/testdb
//	URL:
//	Notes:
func parsePasswordField(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		// strings.CutPrefix is available in Go 1.20+; we use HasPrefix + TrimPrefix
		// to stay compatible with Go 1.22 (which has CutPrefix, so either works).
		if strings.HasPrefix(line, "Password:") {
			value := strings.TrimPrefix(line, "Password:")
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("keepassxc-cli output did not contain a Password field")
}

// isNotFoundText checks whether the error output indicates a missing entry
// rather than a genuine failure (wrong password, corrupt db, etc.).
func isNotFoundText(parts ...any) bool {
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch v := part.(type) {
		case string:
			b.WriteString(v)
		case error:
			b.WriteString(v.Error())
		default:
			b.WriteString(fmt.Sprint(v))
		}
	}
	lower := strings.ToLower(b.String())
	needles := []string{
		"not found",
		"no such entry",
		"entry not found",
		"could not find",
		"no entry",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
