package search_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func m8CorpusDigest(root string, docs []doc) (string, error) {
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, d.name)
	}
	sort.Strings(names)
	h := sha256.New()
	byName := map[string][]byte{}
	for _, d := range docs {
		byName[d.name] = d.payload
	}
	for _, name := range names {
		if filepath.Base(name) != name {
			return "", errM8Rows
		}
		b, err := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // G304: corpus basenames, never label-selected paths
		if err != nil {
			return "", err
		}
		var capture struct {
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(b, &capture) != nil || !bytes.Equal(capture.Payload, byName[name]) {
			return "", errM8Rows
		}
		_, _ = fmt.Fprintf(h, "%s\x00%x\n", name, sha256.Sum256(b))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func m8CheckCorpusBinding(text, digest string) error {
	const prefix = "# M8 corpus SHA-256:"
	seen := false
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		if seen || strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix)) != digest {
			return errM8Rows
		}
		seen = true
	}
	if !seen {
		return errM8Rows
	}
	return nil
}

func TestM8LabelsAreBoundToTheCorpus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "capture.json")
	if err := os.WriteFile(path, []byte(`{"payload":{},"revision":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	docs := []doc{{name: "capture.json", payload: []byte(`{}`)}}
	before, err := m8CorpusDigest(root, docs)
	if err != nil {
		t.Fatal(err)
	}
	header := "# M8 corpus SHA-256: " + before
	if err := m8CheckCorpusBinding(header, before); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"payload":{},"revision":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := m8CorpusDigest(root, docs)
	if err != nil {
		t.Fatal(err)
	}
	if err := m8CheckCorpusBinding(header, after); !errors.Is(err, errM8Rows) {
		t.Fatal("changed corpus accepted")
	}
	if err := m8CheckCorpusBinding("", after); !errors.Is(err, errM8Rows) {
		t.Fatal("missing binding accepted")
	}
	if err := m8CheckCorpusBinding(header+"\n"+header, before); !errors.Is(err, errM8Rows) {
		t.Fatal("duplicate binding accepted")
	}
	if err := os.WriteFile(path, []byte(`{"payload":{"changed":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m8CorpusDigest(root, docs); !errors.Is(err, errM8Rows) {
		t.Fatal("digest was not bound to the payload being evaluated")
	}
}

var (
	errM8Pending = errors.New("M8 labels are not evaluated")
	errM8Source  = errors.New("M8 label source mismatch")
	errM8Rows    = errors.New("invalid or duplicate M8 label rows")
)

type m8LabelKey struct{ failure, candidate string }

func TestM8LabelProvenanceAndCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name, input, source string
		want                error
	}{
		{"agent", "# M8 label source: agent\nf\tc\tyes\tcommand\n", "agent", nil},
		{"owner", "# M8 label source: owner\nf\tc\tno\tcommand\n", "owner", nil},
		{"unknown stays separate", "# M8 label source: agent\nf\tc\tunknown\tcommand\n", "agent", nil},
		{"pending old fixture", "f\tc\tTODO\tcommand\n", "owner", errM8Pending},
		{"pending new fixture", "# M8 label source: unlabelled\nf\tc\tTODO\tcommand\n", "owner", errM8Pending},
		{"agent is not owner", "# M8 label source: agent\nf\tc\tyes\tcommand\n", "owner", errM8Source},
		{"missing source", "f\tc\tyes\tcommand\n", "agent", errM8Source},
		{"duplicate source", "# M8 label source: agent\n# M8 label source: agent\nf\tc\tyes\tcommand\n", "agent", errM8Source},
		{"duplicate pair", "# M8 label source: agent\nf\tc\tyes\tcommand\nf\tc\tno\tcommand\n", "agent", errM8Rows},
		{"bad label", "# M8 label source: agent\nf\tc\tmaybe\tcommand\n", "agent", errM8Rows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := m8ParseLabels(strings.NewReader(tc.input), tc.source)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			if tc.want == nil && len(got) != 1 {
				t.Fatalf("rows=%d", len(got))
			}
			if tc.name == "unknown stays separate" && got[m8LabelKey{"f", "c"}] != "unknown" {
				t.Fatal("unknown label was changed")
			}
		})
	}
}

func m8ParseLabels(r io.Reader, source string) (map[m8LabelKey]string, error) {
	if source != "owner" && source != "agent" {
		return nil, errM8Source
	}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 4096), 4<<20)
	out := map[m8LabelKey]string{}
	declared := ""
	pending := false
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# M8 label source:") {
			if declared != "" {
				return nil, errM8Source
			}
			declared = strings.TrimSpace(strings.TrimPrefix(line, "# M8 label source:"))
			if declared != source && declared != "unlabelled" {
				return nil, errM8Source
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(scan.Text(), "\t")
		if len(fields) != 4 || fields[0] == "" || fields[1] == "" {
			return nil, errM8Rows
		}
		key := m8LabelKey{fields[0], fields[1]}
		if _, ok := out[key]; ok {
			return nil, errM8Rows
		}
		label := strings.ToLower(strings.TrimSpace(fields[2]))
		switch label {
		case "yes", "no", "unknown":
		case "todo":
			pending = true
		default:
			return nil, errM8Rows
		}
		out[key] = label
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if pending || len(out) == 0 {
		return nil, errM8Pending
	}
	if declared != source {
		return nil, errM8Source
	}
	return out, nil
}
