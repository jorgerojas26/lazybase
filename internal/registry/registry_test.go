package registry

import (
	"path/filepath"
	"testing"

	"lazybase/internal/ports"
)

func TestGetOrAllocateReusesExistingSlot(t *testing.T) {
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return true })
	defer restore()

	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{"/tmp/project-a": {Slot: 2}}}

	slot, reused, err := store.GetOrAllocate(reg, "/tmp/project-a", ports.Settings{Offset: ports.DefaultOffset})
	if err != nil {
		t.Fatalf("allocate slot: %v", err)
	}
	if !reused {
		t.Fatal("expected existing slot to be reused")
	}
	if slot != 2 {
		t.Fatalf("expected slot 2, got %d", slot)
	}
}

func TestGetOrAllocateSkipsUsedAndOccupiedSlots(t *testing.T) {
	busy := map[int]bool{
		54325: true,
	}
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return !busy[port] })
	defer restore()

	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{"/tmp/project-a": {Slot: 1}}}

	slot, reused, err := store.GetOrAllocate(reg, "/tmp/project-b", ports.Settings{Offset: ports.DefaultOffset})
	if err != nil {
		t.Fatalf("allocate slot: %v", err)
	}
	if reused {
		t.Fatal("expected a new slot allocation")
	}
	if slot != 2 {
		t.Fatalf("expected slot 2, got %d", slot)
	}
}

func TestGetOrAllocateChecksFullManagedPortSet(t *testing.T) {
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool {
		return port != 54325
	})
	defer restore()

	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}}

	slot, reused, err := store.GetOrAllocate(reg, "/tmp/project-c", ports.Settings{Offset: ports.DefaultOffset})
	if err != nil {
		t.Fatalf("allocate slot: %v", err)
	}
	if reused {
		t.Fatal("expected a new slot allocation")
	}
	if slot != 1 {
		t.Fatalf("expected slot 1 because slot 0 full set is occupied, got %d", slot)
	}
}

func TestGetOrAllocateAssignsDistinctSlotsAndPorts(t *testing.T) {
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return true })
	defer restore()

	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}}
	settings := ports.Settings{}

	firstSlot, reused, err := store.GetOrAllocate(reg, "/tmp/project-a", settings)
	if err != nil {
		t.Fatalf("allocate first slot: %v", err)
	}
	if reused {
		t.Fatal("expected first project to allocate a new slot")
	}
	if firstSlot != 0 {
		t.Fatalf("expected first slot 0, got %d", firstSlot)
	}

	secondSlot, reused, err := store.GetOrAllocate(reg, "/tmp/project-b", settings)
	if err != nil {
		t.Fatalf("allocate second slot: %v", err)
	}
	if reused {
		t.Fatal("expected second project to allocate a new slot")
	}
	if secondSlot != 1 {
		t.Fatalf("expected second slot 1, got %d", secondSlot)
	}

	keys := []ports.PortKey{ports.KeyAPIPort, ports.KeyStudioPort}
	firstPorts := ports.Compute(settings, firstSlot, keys)
	secondPorts := ports.Compute(settings, secondSlot, keys)

	if firstPorts[ports.KeyAPIPort] != 54321 || firstPorts[ports.KeyStudioPort] != 54323 {
		t.Fatalf("expected first project base ports, got %v", firstPorts)
	}
	if secondPorts[ports.KeyAPIPort] != 54321+ports.DefaultOffset || secondPorts[ports.KeyStudioPort] != 54323+ports.DefaultOffset {
		t.Fatalf("expected second project shifted ports, got %v", secondPorts)
	}
	if firstPorts[ports.KeyAPIPort] == secondPorts[ports.KeyAPIPort] || firstPorts[ports.KeyStudioPort] == secondPorts[ports.KeyStudioPort] {
		t.Fatalf("expected distinct ports for distinct slots, got first=%v second=%v", firstPorts, secondPorts)
	}
}

func TestRegistryReadWriteAndPrune(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	store := NewStore(path)

	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{
		"/tmp/project-a": {Slot: 0},
		"/tmp/project-b": {Slot: 2},
	}}
	if err := store.Save(reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(loaded.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(loaded.Projects))
	}

	if !store.Prune(loaded, "/tmp/project-a") {
		t.Fatal("expected prune to remove project")
	}
	if err := store.Save(loaded); err != nil {
		t.Fatalf("save pruned registry: %v", err)
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	if len(reloaded.Projects) != 1 {
		t.Fatalf("expected 1 project after prune, got %d", len(reloaded.Projects))
	}
	if _, ok := reloaded.Projects["/tmp/project-a"]; ok {
		t.Fatal("expected project-a to be removed")
	}
}
