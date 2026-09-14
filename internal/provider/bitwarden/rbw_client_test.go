package bitwarden

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"dotenv-sync/internal/testutil"
)

func TestRBWClientGetRawItemParsesJSON(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status:        "unlocked",
		SuccessStderr: "provider warning",
		Items: map[string]testutil.RBWStubItem{
			"repo": {
				Notes:    `{"format":"dotenv-sync/note-json@v1","env":{}}`,
				Password: "keep-me",
			},
		},
	})
	stub.SetEnv(t)

	item, err := (&RBWClient{Bin: stub.Path()}).GetRawItem(context.Background(), "repo")
	if err != nil {
		t.Fatalf("get raw item: %v", err)
	}
	if item.Name != "repo" || item.Password != "keep-me" {
		t.Fatalf("unexpected raw item: %+v", item)
	}
}

func TestInteractiveScriptArgsUseNativeUnixSyntax(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want []string
	}{
		{
			name: "linux",
			goos: "linux",
			want: []string{"-q", "-c", "'rbw' 'edit' 'repo'", "/dev/null"},
		},
		{
			name: "darwin",
			goos: "darwin",
			want: []string{"-q", "/dev/null", "rbw", "edit", "repo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := interactiveScriptArgs(tc.goos, "rbw", "edit", "repo"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("interactive script args = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRBWClientMutationsUseScriptedEditor(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{
		Status: "unlocked",
		Items: map[string]testutil.RBWStubItem{
			"repo": {},
		},
	})
	stub.SetEnv(t)

	client := &RBWClient{Bin: stub.Path()}
	if err := client.AddItem(context.Background(), "repo", "", `{"env":{}}`); err != nil {
		t.Fatalf("add item: %v", err)
	}
	if err := client.EditItem(context.Background(), "repo", "secret-password", `{"env":{"A":"1"}}`); err != nil {
		t.Fatalf("edit item: %v", err)
	}

	if got := stub.Note(t, "repo"); got != `{"env":{"A":"1"}}` {
		t.Fatalf("unexpected notes after edit: %q", got)
	}
	if got := stub.Password(t, "repo"); got != "secret-password" {
		t.Fatalf("unexpected password after edit: %q", got)
	}
	logData := stub.Log(t)
	if !strings.Contains(logData, "add repo") || !strings.Contains(logData, "edit repo") {
		t.Fatalf("unexpected rbw log: %s", logData)
	}
}

func TestRBWClientGetRawItemTreatsNoEntryFoundAsMissing(t *testing.T) {
	stub := testutil.WriteRBWStub(t, testutil.RBWStubOptions{Status: "unlocked"})
	stub.SetEnv(t)

	_, err := (&RBWClient{Bin: stub.Path()}).GetRawItem(context.Background(), "repo")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}
