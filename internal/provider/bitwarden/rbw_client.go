package bitwarden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrBinaryMissing = errors.New("rbw binary missing")
var ErrItemNotFound = errors.New("rbw item not found")

type RawItem struct {
	Name     string
	Notes    string
	Password string
}

type RBWClient struct {
	Bin string
}

func NewRBWClient() *RBWClient {
	return &RBWClient{Bin: "rbw"}
}

func (c *RBWClient) Run(ctx context.Context, args ...string) (string, error) {
	return c.run(ctx, nil, args...)
}

func (c *RBWClient) run(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	if _, err := exec.LookPath(c.Bin); err != nil {
		return "", ErrBinaryMissing
	}
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	stdoutText := strings.TrimSpace(stdout.String())
	stderrText := strings.TrimSpace(stderr.String())
	text := strings.TrimSpace(strings.Join([]string{stdoutText, stderrText}, "\n"))
	if err != nil {
		if text == "" {
			return text, fmt.Errorf("rbw %s failed: %w", strings.Join(args, " "), err)
		}
		return text, fmt.Errorf("%s", text)
	}
	return stdoutText, nil
}

func (c *RBWClient) GetRawItem(ctx context.Context, itemName string) (RawItem, error) {
	out, err := c.Run(ctx, "get", "--raw", itemName)
	if err != nil {
		if isNotFoundText(out, err) {
			return RawItem{}, ErrItemNotFound
		}
		return RawItem{}, err
	}
	var payload struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
		Data  struct {
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return RawItem{}, fmt.Errorf("parse rbw raw item: %w", err)
	}
	return RawItem{Name: payload.Name, Notes: payload.Notes, Password: payload.Data.Password}, nil
}

func (c *RBWClient) AddItem(ctx context.Context, itemName, password, notes string) error {
	return c.mutateItem(ctx, "add", itemName, password, notes)
}

func (c *RBWClient) EditItem(ctx context.Context, itemName, password, notes string) error {
	return c.mutateItem(ctx, "edit", itemName, password, notes)
}

func (c *RBWClient) Sync(ctx context.Context) error {
	_, err := c.Run(ctx, "sync")
	return err
}

func (c *RBWClient) mutateItem(ctx context.Context, commandName, itemName, password, notes string) error {
	workDir, err := os.MkdirTemp("", "ds-rbw-editor-*")
	if err != nil {
		return fmt.Errorf("create editor work dir: %w", err)
	}
	defer os.RemoveAll(workDir)
	sourcePath := filepath.Join(workDir, "content.txt")
	if err := os.WriteFile(sourcePath, []byte(renderEditorContent(password, notes)), 0o600); err != nil {
		return fmt.Errorf("write editor content: %w", err)
	}
	editorPath, err := writeEditorScript(workDir)
	if err != nil {
		return err
	}
	_, err = c.runInteractive(ctx, []string{
		"DS_RBW_EDITOR_SOURCE=" + sourcePath,
		"EDITOR=" + editorPath,
		"VISUAL=" + editorPath,
	}, commandName, itemName)
	if err != nil {
		return err
	}
	return nil
}

func (c *RBWClient) runInteractive(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	if runtime.GOOS == "windows" {
		return c.run(ctx, extraEnv, args...)
	}
	if _, err := exec.LookPath(c.Bin); err != nil {
		return "", ErrBinaryMissing
	}
	scriptBin, err := exec.LookPath("script")
	if err != nil {
		return c.run(ctx, extraEnv, args...)
	}
	cmd := exec.CommandContext(ctx, scriptBin, interactiveScriptArgs(runtime.GOOS, c.Bin, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return text, fmt.Errorf("rbw %s failed: %w", strings.Join(args, " "), err)
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

func interactiveScriptArgs(goos, bin string, args ...string) []string {
	if goos == "darwin" {
		return append([]string{"-q", "/dev/null", bin}, args...)
	}
	commandLine := shellQuote(bin)
	for _, arg := range args {
		commandLine += " " + shellQuote(arg)
	}
	return []string{"-q", "-c", commandLine, "/dev/null"}
}

func renderEditorContent(password, notes string) string {
	if strings.TrimSpace(notes) == "" {
		return password + "\n"
	}
	return password + "\n\n" + notes + "\n"
}

func writeEditorScript(dir string) (string, error) {
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "editor.cmd")
		content := "@echo off\r\ntype \"%DS_RBW_EDITOR_SOURCE%\" > \"%~1\"\r\n"
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			return "", fmt.Errorf("write editor script: %w", err)
		}
		return path, nil
	}
	path := filepath.Join(dir, "editor.sh")
	content := "#!/bin/sh\ncat \"$DS_RBW_EDITOR_SOURCE\" > \"$1\"\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return "", fmt.Errorf("write editor script: %w", err)
	}
	return path, nil
}

func isNotFoundText(parts ...any) bool {
	var builder strings.Builder
	for i, part := range parts {
		if i > 0 {
			builder.WriteByte(' ')
		}
		switch value := part.(type) {
		case string:
			builder.WriteString(value)
		case error:
			builder.WriteString(value.Error())
		default:
			builder.WriteString(fmt.Sprint(value))
		}
	}
	lower := strings.ToLower(builder.String())
	needles := []string{
		"not found",
		"no item",
		"missing",
		"no entry found",
		"no such entry",
		"couldn't find entry",
		"could not find entry",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
