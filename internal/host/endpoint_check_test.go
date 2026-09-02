package host

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPointsAtEndpointReadsTheHostsOwnFile pins the three answers `install`
// acts on: no file is a host to register, a file naming the URL is one that
// already is, and a file naming engramux at another URL is stale and gets
// registered again.
func TestPointsAtEndpointReadsTheHostsOwnFile(t *testing.T) {
	const url = "http://127.0.0.1:8867/mcp"
	dir := t.TempDir()

	if ok, err := PointsAtEndpoint(filepath.Join(dir, "absent.json"), url); err != nil || ok {
		t.Errorf("an absent file answered %v, %v; want false, nil", ok, err)
	}
	if ok, err := PointsAtEndpoint("", url); err != nil || ok {
		t.Errorf("an empty path answered %v, %v; want false, nil", ok, err)
	}

	registered := filepath.Join(dir, "registered.json")
	if err := os.WriteFile(registered, []byte(`{"mcpServers":{"engramux":{"type":"http","url":"`+url+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := PointsAtEndpoint(registered, url); err != nil || !ok {
		t.Errorf("a file naming the endpoint answered %v, %v; want true, nil", ok, err)
	}

	stale := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(stale, []byte(`{"mcpServers":{"engramux":{"type":"http","url":"http://127.0.0.1:9999/mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := PointsAtEndpoint(stale, url); err != nil || ok {
		t.Errorf("a file naming another endpoint answered %v, %v; want false, nil", ok, err)
	}
}
