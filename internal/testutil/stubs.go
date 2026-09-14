package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	helperModeEnv  = "DS_TEST_HELPER"
	helperStateEnv = "DS_TEST_HELPER_STATE"
)

type RBWStubItem struct {
	Notes    string `json:"notes"`
	Password string `json:"password"`
}

type RBWStubOptions struct {
	Status               string                 `json:"status"`
	Fields               map[string]string      `json:"fields"`
	Missing              []string               `json:"missing"`
	Items                map[string]RBWStubItem `json:"items"`
	LegacyStatusFallback bool                   `json:"legacy_status_fallback"`
	SuccessStderr        string                 `json:"success_stderr"`
}

type RBWStub struct {
	path    string
	env     []string
	logFile string
	items   string
}

func WriteRBWStub(t testing.TB, opts RBWStubOptions) RBWStub {
	t.Helper()
	stubDir := t.TempDir()
	itemsDir := filepath.Join(stubDir, "items")
	logFile := filepath.Join(stubDir, "rbw.log")
	if err := os.MkdirAll(itemsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for item, state := range opts.Items {
		writeTestFile(t, filepath.Join(itemsDir, item+".notes"), state.Notes)
		writeTestFile(t, filepath.Join(itemsDir, item+".password"), state.Password)
	}
	statePath := writeState(t, stubDir, "rbw", rbwHelperState{
		RBWStubOptions: opts,
		ItemsDir:       itemsDir,
		LogFile:        logFile,
	})
	helper := installHelper(t, stubDir, "rbw", statePath)
	return RBWStub{path: helper.path, env: helper.env, logFile: logFile, items: itemsDir}
}

func (s RBWStub) Path() string {
	return s.path
}

func (s RBWStub) Env() []string {
	return append([]string{}, s.env...)
}

func (s RBWStub) SetEnv(t testing.TB) {
	t.Helper()
	for _, entry := range s.env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			t.Setenv(key, value)
		}
	}
}

func (s RBWStub) Log(t testing.TB) string {
	t.Helper()
	data, err := os.ReadFile(s.logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

func (s RBWStub) Note(t testing.TB, item string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.items, item+".notes"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (s RBWStub) Password(t testing.TB, item string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.items, item+".password"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type KeepassStubOptions struct {
	Entries map[string][]string `json:"entries"`
	Values  map[string]string   `json:"values"`
	Missing []string            `json:"missing"`
}

type KeepassStub struct {
	path string
	env  []string
}

func WriteKeepassStub(t testing.TB, opts KeepassStubOptions) KeepassStub {
	t.Helper()
	stubDir := t.TempDir()
	statePath := writeState(t, stubDir, "keepass", opts)
	helper := installHelper(t, stubDir, "keepass", statePath)
	return KeepassStub{path: helper.path, env: helper.env}
}

func (s KeepassStub) Path() string {
	return s.path
}

func (s KeepassStub) Env() []string {
	return append([]string{}, s.env...)
}

func (s KeepassStub) SetEnv(t testing.TB) {
	t.Helper()
	for _, entry := range s.env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			t.Setenv(key, value)
		}
	}
}

type rbwHelperState struct {
	RBWStubOptions
	ItemsDir string `json:"items_dir"`
	LogFile  string `json:"log_file"`
}

type helperStub struct {
	path string
	env  []string
}

func installHelper(t testing.TB, stubDir, mode, statePath string) helperStub {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := mode
	if mode == "keepass" {
		name = "keepassxc-cli"
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(stubDir, name)
	copyTestExecutable(t, executable, path)
	env := []string{
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		helperModeEnv + "=" + mode,
		helperStateEnv + "=" + statePath,
	}
	return helperStub{path: path, env: env}
}

func writeState(t testing.TB, dir, name string, state any) string {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+"-state.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyTestExecutable(t testing.TB, source, target string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func RunHelperProcess() (bool, int) {
	switch os.Getenv(helperModeEnv) {
	case "rbw":
		return true, runRBWHelper()
	case "keepass":
		return true, runKeepassHelper()
	default:
		return false, 0
	}
}

func readHelperState(target any) error {
	path := os.Getenv(helperStateEnv)
	if path == "" {
		return fmt.Errorf("%s is not set", helperStateEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func runRBWHelper() int {
	var state rbwHelperState
	if err := readHelperState(&state); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	args := os.Args[1:]
	if len(args) > 0 {
		if err := appendLog(state.LogFile, args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing rbw command")
		return 1
	}
	if state.SuccessStderr != "" {
		fmt.Fprintln(os.Stderr, state.SuccessStderr)
	}

	switch args[0] {
	case "unlocked":
		if state.LegacyStatusFallback {
			fmt.Fprintln(os.Stderr, "error: unrecognized subcommand 'unlocked'")
			return 2
		}
		if state.Status == "unlocked" {
			return 0
		}
		return 1
	case "list":
		switch state.Status {
		case "unlocked":
			fmt.Println("DATABASE_URL\nJWT_SECRET")
			return 0
		case "locked":
			fmt.Fprintln(os.Stderr, "database is locked")
		case "logged out":
			fmt.Fprintln(os.Stderr, "not logged in")
		default:
			fmt.Println(state.Status)
			return 0
		}
		return 1
	case "status":
		fmt.Println(state.Status)
		return 0
	case "get":
		return runRBWGet(state, args[1:])
	case "add", "edit":
		return runRBWMutation(state, args[0], args[1:])
	case "sync":
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unsupported rbw command")
		return 1
	}
}

func runRBWGet(state rbwHelperState, args []string) int {
	if len(args) >= 2 && args[0] == "--raw" {
		item := args[1]
		passwordPath := filepath.Join(state.ItemsDir, item+".password")
		notesPath := filepath.Join(state.ItemsDir, item+".notes")
		if !fileExists(passwordPath) && !fileExists(notesPath) {
			fmt.Fprintln(os.Stderr, "not found")
			return 1
		}
		password := readOptionalFile(passwordPath)
		notes := readOptionalFile(notesPath)
		encoded, err := json.Marshal(map[string]any{
			"name":  item,
			"notes": notes,
			"data":  map[string]string{"password": password},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	field := ""
	item := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--field":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "missing field")
				return 1
			}
			field = args[i+1]
			i++
		case "--raw":
			i++
		default:
			item = args[i]
		}
	}
	if field == "password" {
		path := filepath.Join(state.ItemsDir, item+".password")
		if fileExists(path) {
			fmt.Println(readOptionalFile(path))
			return 0
		}
	}
	key := item + "::" + field
	if value, ok := state.Fields[key]; ok {
		fmt.Println(value)
		return 0
	}
	for _, missing := range state.Missing {
		if missing == key {
			fmt.Fprintln(os.Stderr, "not found")
			return 1
		}
	}
	fmt.Fprintln(os.Stderr, "not found")
	return 1
}

func runRBWMutation(state rbwHelperState, command string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing item")
		return 1
	}
	item := args[0]
	passwordPath := filepath.Join(state.ItemsDir, item+".password")
	notesPath := filepath.Join(state.ItemsDir, item+".notes")
	if command == "edit" && !fileExists(passwordPath) && !fileExists(notesPath) {
		fmt.Fprintln(os.Stderr, "not found")
		return 1
	}
	temp, err := os.CreateTemp("", "ds-rbw-helper-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.Remove(tempPath)
	if source := os.Getenv("DS_RBW_EDITOR_SOURCE"); source != "" {
		data, err := os.ReadFile(source)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(tempPath, data, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			fmt.Fprintln(os.Stderr, "editor is not set")
			return 1
		}
		if err := exec.Command(editor, tempPath).Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	content := strings.ReplaceAll(readOptionalFile(tempPath), "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	password := ""
	if len(lines) > 0 {
		password = lines[0]
	}
	rest := lines[1:]
	for len(rest) > 0 && rest[0] == "" {
		rest = rest[1:]
	}
	notes := make([]string, 0, len(rest))
	for _, line := range rest {
		if !strings.HasPrefix(line, "#") {
			notes = append(notes, line)
		}
	}
	if err := os.WriteFile(passwordPath, []byte(password), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(notesPath, []byte(strings.Join(notes, "\n")), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runKeepassHelper() int {
	var state KeepassStubOptions
	if err := readHelperState(&state); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing keepassxc-cli command")
		return 1
	}
	switch args[0] {
	case "show":
		entry := args[len(args)-1]
		for _, missing := range state.Missing {
			if missing == entry {
				fmt.Fprintln(os.Stderr, "Entry not found.")
				return 1
			}
		}
		value, ok := state.Values[entry]
		if !ok {
			fmt.Fprintln(os.Stderr, "Entry not found.")
			return 1
		}
		fmt.Printf("Title: %s\nUserName: \nPassword: %s\nURL: \nNotes: \n", filepath.Base(entry), value)
		return 0
	case "ls":
		group := args[len(args)-1]
		for _, missing := range state.Missing {
			if missing == group {
				fmt.Fprintln(os.Stderr, "Could not find entry "+group+".")
				return 1
			}
		}
		fmt.Println(strings.Join(state.Entries[group], "\n"))
		return 0
	case "add":
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unsupported keepassxc-cli command")
		return 1
	}
}

func appendLog(path string, args []string) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, strings.Join(args, " "))
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readOptionalFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
