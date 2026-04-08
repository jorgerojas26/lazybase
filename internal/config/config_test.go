package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lazybase/internal/ports"
)

func TestPatchPreservesTextAndComments(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"# top comment",
		"[api]",
		"port = 54321 # api comment",
		"",
		"[db]",
		"# port = 15432",
		"port = 54322",
		"shadow_port = 54320",
		"",
		"[db.pooler]",
		"port = 54329 # pooler",
		"",
		"[inbucket]",
		"port = 54324",
		"# smtp_port = 54325",
		"pop3_port = 54326",
		"",
		"[analytics]",
		"port = 54327",
		"",
		"[edge_runtime]",
		"# inspector_port = 8083",
		"",
		"[unrelated]",
		"value = \"keep me\"",
	}, "\n") + "\n")

	target := ports.PortMap{
		ports.KeyAPIPort:          54421,
		ports.KeyDBPort:           54422,
		ports.KeyDBShadowPort:     54420,
		ports.KeyDBPoolerPort:     54429,
		ports.KeyInbucketPort:     54424,
		ports.KeyInbucketPOP3Port: 54426,
		ports.KeyAnalyticsPort:    54427,
	}

	patched, changed := PatchRawForTests(raw, target)
	if !changed {
		t.Fatal("expected patch to report changes")
	}

	text := string(patched)
	assertContains(t, text, "port = 54421 # api comment")
	assertContains(t, text, "port = 54422")
	assertContains(t, text, "shadow_port = 54420")
	assertContains(t, text, "port = 54429 # pooler")
	assertContains(t, text, "port = 54424")
	assertContains(t, text, "pop3_port = 54426")
	assertContains(t, text, "port = 54427")
	assertContains(t, text, "# smtp_port = 54325")
	assertContains(t, text, "# inspector_port = 8083")
	assertContains(t, text, "value = \"keep me\"")
}

func TestReadFileRejectsCorruptTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[api\nport = 54321\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := ReadFile(path); err == nil {
		t.Fatal("expected TOML validation error")
	}
}

func TestPatchCreatesBackupOnlyOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	raw := []byte("[api]\nport = 54321\n[studio]\nport = 54323\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	file, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	changed, err := file.Patch(ports.PortMap{ports.KeyAPIPort: 54421, ports.KeyStudioPort: 54423})
	if err != nil {
		t.Fatalf("patch file: %v", err)
	}
	if !changed {
		t.Fatal("expected config to change")
	}

	backupPath := filepath.Join(dir, ".config.toml.bak")
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != string(raw) {
		t.Fatalf("unexpected backup contents: %q", string(backup))
	}

	changed, err = file.Patch(ports.PortMap{ports.KeyAPIPort: 54421, ports.KeyStudioPort: 54423})
	if err != nil {
		t.Fatalf("patch file second time: %v", err)
	}
	if changed {
		t.Fatal("expected second patch to be a no-op")
	}
}

func assertContains(t *testing.T, text, fragment string) {
	t.Helper()
	if !strings.Contains(text, fragment) {
		t.Fatalf("expected %q to contain %q", text, fragment)
	}
}
