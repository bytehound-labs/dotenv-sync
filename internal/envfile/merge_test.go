package envfile

import (
	"strings"
	"testing"
)

func TestInitSchemaFromEnvBlanksSecretLikeValues(t *testing.T) {
	local := ParseBytes(".env", KindLocal, []byte("DATABASE_URL=postgres://localhost/dev\nPORT=8080\n"))
	schema := InitSchemaFromEnv(local)
	got := string(Render(schema))
	if !strings.Contains(got, "DATABASE_URL=\n") || !strings.Contains(got, "PORT=8080") {
		t.Fatalf("unexpected schema output: %s", got)
	}
}

func TestInitSchemaFromEnvBlanksConfiguredLocalValues(t *testing.T) {
	local := ParseBytes(".env", KindLocal, []byte("FILE=1\nPORT=8080\n"))
	schema := InitSchemaFromEnvWithLocalKeys(local, func(key string) bool { return key == "FILE" })
	got := string(Render(schema))
	if got != "FILE=\nPORT=8080\n" {
		t.Fatalf("unexpected schema output: %s", got)
	}
}

func TestReverseMergeAddsBlankPlaceholders(t *testing.T) {
	schema := ParseBytes(".env.example", KindSchema, []byte("PORT=8080\n"))
	local := ParseBytes(".env", KindLocal, []byte("PORT=8080\nAPI_KEY=value\n"))
	result, added := ReverseMerge(schema, local)
	if len(added) != 1 || added[0] != "API_KEY" {
		t.Fatalf("unexpected added keys: %#v", added)
	}
	if !strings.Contains(string(Render(result)), "API_KEY=\n") {
		t.Fatalf("expected blank placeholder, got: %s", Render(result))
	}
}
