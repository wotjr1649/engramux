package inject_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

var (
	errM7Pending = errors.New("M7 labels are pending; no evaluation was performed")
	errM7Source  = errors.New("M7 label source is missing, conflicting, or incompatible with this evaluation")
)

const (
	m7Owner            = "owner"
	m7Agent            = "agent"
	m7SourcePrefix     = "# M7 label source:"
	m7UnlabelledHeader = m7SourcePrefix + " unlabelled"
)

// m7ParseLabels checks authorship before accepting a scoreable fixture. A path
// override changes where a fixture lives, never who supplied its judgements.
// The legacy agent header remains readable so the first proxy run can be
// archived byte-for-byte. Metadata records provenance, not activation consent.
func m7ParseLabels(r io.Reader, fields int, source string) ([][]string, error) {
	if (fields != 4 && fields != 6) || (source != m7Owner && source != m7Agent) {
		return nil, fmt.Errorf("invalid M7 reader configuration")
	}
	var out [][]string
	var declared string
	var sourceSeen, pending bool
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		value, found := strings.CutPrefix(line, m7SourcePrefix)
		if strings.HasPrefix(line, "# Label provenance: Codex agent estimates,") {
			value, found = m7Agent, true
		}
		if found {
			if sourceSeen {
				return nil, errM7Source
			}
			sourceSeen, declared = true, strings.TrimSpace(value)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != fields {
			return nil, fmt.Errorf("M7 row has %d columns, want %d", len(parts), fields)
		}
		switch strings.ToLower(strings.TrimSpace(parts[fields-2])) {
		case m7Yes, m7No:
		case "todo":
			pending = true
		default:
			return nil, fmt.Errorf("M7 row %d has an invalid label; use yes, no or TODO", len(out)+1)
		}
		out = append(out, parts)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	switch declared {
	case m7Owner, m7Agent:
		if declared != source {
			return nil, errM7Source
		}
	case "", "unlabelled":
		if !pending && len(out) > 0 {
			return nil, errM7Source
		}
	default:
		return nil, errM7Source
	}
	if pending || len(out) == 0 {
		return nil, errM7Pending
	}
	return out, nil
}

func TestM7LabelsRequireTheirDeclaredSource(t *testing.T) {
	const prompt = "p1\tlatin\tyes\tsynthetic prompt\n"
	const block = "p1\tb1\tevent\t12\tno\tsynthetic block\n"
	for _, tc := range []struct {
		name, header, source string
		wantErr              error
	}{
		{"owner", "# M7 label source: owner\n", "owner", nil},
		{"agent", "# M7 label source: agent\n", "agent", nil},
		{"agent cannot enter owner gate", "# M7 label source: agent\n", "owner", errM7Source},
		{"owner cannot be reported as agent", "# M7 label source: owner\n", "agent", errM7Source},
		{"complete labels need a source", "", "owner", errM7Source},
		{"unknown source", "# M7 label source: guessed\n", "owner", errM7Source},
		{"unlabelled is not owner consent", "# M7 label source: unlabelled\n", "owner", errM7Source},
		{"legacy agent archive", "# Label provenance: Codex agent estimates, user-requested 2026-09-08; NOT owner judgements.\n", "agent", nil},
		{"legacy agent is not owner", "# Label provenance: Codex agent estimates, user-requested 2026-09-08; NOT owner judgements.\n", "owner", errM7Source},
		{"conflicting headers", "# M7 label source: owner\n# M7 label source: agent\n", "owner", errM7Source},
		{"duplicate headers", "# M7 label source: owner\n# M7 label source: owner\n", "owner", errM7Source},
		{"legacy marker cannot be relabelled by another header", "# Label provenance: Codex agent estimates, archived.\n# M7 label source: owner\n", "owner", errM7Source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, row := range []string{prompt, block} {
				fields := len(strings.Split(strings.TrimSuffix(row, "\n"), "\t"))
				got, err := m7ParseLabels(strings.NewReader(tc.header+row), fields, tc.source)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if err == nil && (len(got) != 1 || strings.Join(got[0], "\t")+"\n" != row) {
					t.Fatal("accepted rows did not preserve the label and data fields")
				}
			}
		})
	}
}

func TestM7PendingLabelsAreNotAResult(t *testing.T) {
	for _, header := range []string{"", "# M7 label source: unlabelled\n", "# M7 label source: owner\n"} {
		_, err := m7ParseLabels(strings.NewReader(header+"p1\tlatin\tTODO\tsynthetic prompt\n"), 4, "owner")
		if !errors.Is(err, errM7Pending) {
			t.Fatalf("error = %v, want pending", err)
		}
	}
	_, err := m7ParseLabels(strings.NewReader("# M7 label source: agent\np1\tlatin\tTODO\tsynthetic prompt\n"), 4, "owner")
	if !errors.Is(err, errM7Source) {
		t.Fatalf("wrong-source TODO: error = %v, want source rejection", err)
	}
	_, err = m7ParseLabels(strings.NewReader(""), 4, "owner")
	if !errors.Is(err, errM7Pending) {
		t.Fatalf("empty file: error = %v, want pending", err)
	}
}

func TestM7InvalidLabelsFailWithoutEchoingTheirValue(t *testing.T) {
	const privateValue = "private-value-must-not-appear"
	_, err := m7ParseLabels(strings.NewReader("# M7 label source: owner\np1\tlatin\t"+privateValue+"\tsynthetic prompt\n"), 4, "owner")
	if err == nil || errors.Is(err, errM7Pending) || strings.Contains(err.Error(), privateValue) {
		t.Fatal("invalid label must fail, not skip or expose its value")
	}
	_, err = m7ParseLabels(strings.NewReader("# M7 label source: owner\np1\tyes\n"), 4, "owner")
	if err == nil || errors.Is(err, errM7Pending) {
		t.Fatal("malformed rows must fail")
	}
}
