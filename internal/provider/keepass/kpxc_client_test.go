package keepass

import (
	"context"
	"errors"
	"testing"

	"dotenv-sync/internal/testutil"
)

func clientWithStub(stub testutil.KeepassStub, password string) *KPXCClient {
	return &KPXCClient{Bin: stub.Path(), Password: password}
}

// --- parsePasswordField ---

func TestParsePasswordFieldExtractsValue(t *testing.T) {
	output := "Title: DATABASE_URL\nUserName: \nPassword: postgres://localhost:5432/testdb\nURL: \nNotes: \n"
	got, err := parsePasswordField(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "postgres://localhost:5432/testdb" {
		t.Fatalf("expected postgres URL, got %q", got)
	}
}

func TestParsePasswordFieldHandlesEmptyValue(t *testing.T) {
	output := "Title: MY_KEY\nPassword: \nNotes: \n"
	got, err := parsePasswordField(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestParsePasswordFieldErrorsWhenMissing(t *testing.T) {
	output := "Title: MY_KEY\nNotes: no password line here\n"
	_, err := parsePasswordField(output)
	if err == nil {
		t.Fatal("expected error when Password field missing")
	}
}

// --- isNotFoundText ---

func TestIsNotFoundTextMatchesKnownPhrases(t *testing.T) {
	cases := []string{
		"Entry not found.",
		"Could not find entry dotenv/MISSING_KEY",
		"no such entry",
		"no entry for this path",
	}
	for _, c := range cases {
		if !isNotFoundText(c) {
			t.Errorf("expected isNotFoundText(%q) = true", c)
		}
	}
}

func TestIsNotFoundTextIgnoresUnrelatedErrors(t *testing.T) {
	cases := []string{
		"Invalid credentials were provided",
		"HMAC mismatch",
		"database file is locked",
	}
	for _, c := range cases {
		if isNotFoundText(c) {
			t.Errorf("expected isNotFoundText(%q) = false", c)
		}
	}
}

// --- KPXCClient.Show ---

func TestKPXCClientShowReturnsPasswordValue(t *testing.T) {
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Values: map[string]string{"dotenv/DATABASE_URL": "postgres://vault/dev"},
	})
	stub.SetEnv(t)
	value, err := clientWithStub(stub, "masterpassword").Show(context.Background(), "test.kdbx", "dotenv/DATABASE_URL")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if value != "postgres://vault/dev" {
		t.Fatalf("expected postgres URL, got %q", value)
	}
}

func TestKPXCClientShowReturnsErrItemNotFound(t *testing.T) {
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Missing: []string{"dotenv/MISSING"},
	})
	stub.SetEnv(t)
	_, err := clientWithStub(stub, "masterpassword").Show(context.Background(), "test.kdbx", "dotenv/MISSING")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestKPXCClientShowReturnsErrBinaryMissing(t *testing.T) {
	client := &KPXCClient{Bin: "/nonexistent/keepassxc-cli", Password: "pw"}
	_, err := client.Show(context.Background(), "test.kdbx", "dotenv/KEY")
	if !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("expected ErrBinaryMissing, got %v", err)
	}
}

// --- KPXCClient.ListGroup ---

func TestKPXCClientListGroupReturnsEntries(t *testing.T) {
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Entries: map[string][]string{
			"dotenv": {"DATABASE_URL", "JWT_SECRET", "subgroup/"},
		},
	})
	stub.SetEnv(t)
	entries, err := clientWithStub(stub, "masterpassword").ListGroup(context.Background(), "test.kdbx", "dotenv")
	if err != nil {
		t.Fatalf("ListGroup: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0] != "DATABASE_URL" || entries[1] != "JWT_SECRET" {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestKPXCClientListGroupReturnsErrItemNotFound(t *testing.T) {
	stub := testutil.WriteKeepassStub(t, testutil.KeepassStubOptions{
		Missing: []string{"nosuchgroup"},
	})
	stub.SetEnv(t)
	_, err := clientWithStub(stub, "masterpassword").ListGroup(context.Background(), "test.kdbx", "nosuchgroup")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}
