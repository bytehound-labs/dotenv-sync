package sync

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"dotenv-sync/internal/envfile"
)

const NoteJSONFormat = "dotenv-sync/note-json@v1"

type NoteJSONEnvelope struct {
	Format string            `json:"format"`
	Env    map[string]string `json:"env"`
}

func CanonicalEnvMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	maps.Copy(cloned, values)
	return cloned
}

func CanonicalDocumentEnv(doc envfile.Document) map[string]string {
	return CanonicalDocumentEnvExcept(doc, nil)
}

func CanonicalDocumentEnvExcept(doc envfile.Document, exclude func(string) bool) map[string]string {
	values := make(map[string]string, len(doc.Lines))
	for _, line := range doc.Lines {
		if line.LineType != envfile.LineAssignment {
			continue
		}
		if exclude != nil && exclude(line.Key) {
			continue
		}
		values[line.Key] = line.Value
	}
	return CanonicalEnvMap(values)
}

func RenderNoteJSON(values map[string]string) (string, error) {
	payload := NoteJSONEnvelope{
		Format: NoteJSONFormat,
		Env:    CanonicalEnvMap(values),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal note_json payload: %w", err)
	}
	return string(data), nil
}

func ParseNoteJSON(raw string) (NoteJSONEnvelope, error) {
	var payload NoteJSONEnvelope
	if strings.TrimSpace(raw) == "" {
		return payload, fmt.Errorf("missing note_json payload")
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return payload, fmt.Errorf("parse note_json payload: %w", err)
	}
	if strings.TrimSpace(payload.Format) != NoteJSONFormat {
		return payload, fmt.Errorf("unsupported note_json format %q", payload.Format)
	}
	payload.Env = CanonicalEnvMap(payload.Env)
	return payload, nil
}

func NoteJSONEqual(left, right map[string]string) bool {
	return maps.Equal(CanonicalEnvMap(left), CanonicalEnvMap(right))
}
