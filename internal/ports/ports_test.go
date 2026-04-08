package ports

import (
	"path/filepath"
	"testing"

	"os"
)

func TestComputeDefaultsUnsetOffsetAndShiftsSlots(t *testing.T) {
	keys := []PortKey{KeyAPIPort, KeyStudioPort}

	slot0 := Compute(Settings{}, 0, keys)
	if slot0[KeyAPIPort] != 54321 || slot0[KeyStudioPort] != 54323 {
		t.Fatalf("expected slot 0 to use base ports, got api=%d studio=%d", slot0[KeyAPIPort], slot0[KeyStudioPort])
	}

	slot1 := Compute(Settings{}, 1, keys)
	if slot1[KeyAPIPort] != 54321+DefaultOffset || slot1[KeyStudioPort] != 54323+DefaultOffset {
		t.Fatalf("expected slot 1 to use default offset, got api=%d studio=%d", slot1[KeyAPIPort], slot1[KeyStudioPort])
	}

	if slot1[KeyAPIPort] == slot0[KeyAPIPort] || slot1[KeyStudioPort] == slot0[KeyStudioPort] {
		t.Fatalf("expected slot 1 ports to differ from slot 0, got slot0=%v slot1=%v", slot0, slot1)
	}
}

func TestLoadSettingsUsesDefaultOffsetWhenMissing(t *testing.T) {
	settings, err := LoadSettings(filepath.Join(t.TempDir(), "lazybase.yaml"))
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings.Offset != DefaultOffset {
		t.Fatalf("expected default offset %d, got %d", DefaultOffset, settings.Offset)
	}
}

func TestLoadSettingsHonorsCustomOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lazybase.yaml")
	if err := os.WriteFile(path, []byte("offset: 37\n"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings.Offset != 37 {
		t.Fatalf("expected custom offset 37, got %d", settings.Offset)
	}

	ports := Compute(settings, 1, []PortKey{KeyAPIPort})
	if ports[KeyAPIPort] != 54321+37 {
		t.Fatalf("expected api port %d, got %d", 54321+37, ports[KeyAPIPort])
	}
}
