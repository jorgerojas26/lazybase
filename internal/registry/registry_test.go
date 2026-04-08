package registry

import (
	"os"
	"path/filepath"
	"testing"

	"lazybase/internal/ports"
	"lazybase/internal/project"
)

func TestGetOrAllocateReusesExistingSlotByProjectID(t *testing.T) {
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return true })
	defer restore()

	root := canonicalTempProject(t)
	info := mustProjectInfo(t, root)
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{info.ID: {ID: info.ID, Path: info.Root, Slot: 2}}}

	slot, reused, err := store.GetOrAllocate(reg, info, ports.Settings{Offset: ports.DefaultOffset})
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

func TestGetOrAllocateMigratesLegacyPathKeyedEntry(t *testing.T) {
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return true })
	defer restore()

	root := canonicalTempProject(t)
	info := mustProjectInfo(t, root)
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}, legacy: map[string]ProjectEntry{info.Root: {Path: info.Root, Slot: 3}}}

	slot, reused, err := store.GetOrAllocate(reg, info, ports.Settings{Offset: ports.DefaultOffset})
	if err != nil {
		t.Fatalf("allocate slot: %v", err)
	}
	if !reused {
		t.Fatal("expected legacy slot to be reused")
	}
	if slot != 3 {
		t.Fatalf("expected slot 3, got %d", slot)
	}
	if _, ok := reg.Projects[info.ID]; !ok {
		t.Fatalf("expected migrated entry under project ID %q", info.ID)
	}
}

func TestGetOrAllocateSkipsUsedAndOccupiedSlots(t *testing.T) {
	busy := map[int]bool{54325: true}
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return !busy[port] })
	defer restore()

	rootA := canonicalTempProject(t)
	rootB := canonicalTempProject(t)
	infoA := mustProjectInfo(t, rootA)
	infoB := mustProjectInfo(t, rootB)
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{infoA.ID: {ID: infoA.ID, Path: infoA.Root, Slot: 1}}}

	slot, reused, err := store.GetOrAllocate(reg, infoB, ports.Settings{Offset: ports.DefaultOffset})
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
	restore := ports.SetAvailabilityCheckerForTests(func(port int) bool { return port != 54325 })
	defer restore()

	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}}

	slot, reused, err := store.GetOrAllocate(reg, mustProjectInfo(t, canonicalTempProject(t)), ports.Settings{Offset: ports.DefaultOffset})
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

func TestRegistryLoadMigratesOldPathKeyedFormat(t *testing.T) {
	root := canonicalTempProject(t)
	store := NewStore(filepath.Join(t.TempDir(), "registry.json"))
	raw := []byte("{\n  \"version\": 1,\n  \"projects\": {\n    \"" + root + "\": {\"slot\": 4}\n  }\n}\n")
	if err := os.WriteFile(store.path, raw, 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	reg, err := store.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if reg.Version != currentVersion {
		t.Fatalf("expected version %d, got %d", currentVersion, reg.Version)
	}
	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(reg.Projects))
	}
	info := mustProjectInfo(t, root)
	entry, ok := reg.Projects[info.ID]
	if !ok {
		t.Fatalf("expected migrated entry with ID %q", info.ID)
	}
	if entry.Path != root || entry.Slot != 4 {
		t.Fatalf("unexpected migrated entry: %+v", entry)
	}

	if err := store.Save(reg); err != nil {
		t.Fatalf("save migrated registry: %v", err)
	}
	saved, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("read saved registry: %v", err)
	}
	if string(saved) == string(raw) {
		t.Fatal("expected saved registry to persist migrated v2 format")
	}
}

func TestRegistryReadWriteListAndPrune(t *testing.T) {
	rootA := canonicalTempProject(t)
	rootB := canonicalTempProject(t)
	infoA := mustProjectInfo(t, rootA)
	infoB := mustProjectInfo(t, rootB)
	path := filepath.Join(t.TempDir(), "registry.json")
	store := NewStore(path)

	reg := &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{
		infoA.ID: {ID: infoA.ID, Path: infoA.Root, Slot: 0},
		infoB.ID: {ID: infoB.ID, Path: infoB.Root, Slot: 2},
	}}
	if err := store.Save(reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	projects := store.List(loaded)
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].RuntimePath != project.RuntimeRoot(projects[0].Path) {
		t.Fatalf("unexpected runtime path %q", projects[0].RuntimePath)
	}

	if !store.Prune(loaded, infoA.ID) {
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
	if _, ok := reloaded.Projects[infoA.ID]; ok {
		t.Fatal("expected project-a to be removed")
	}
}

func canonicalTempProject(t *testing.T) string {
	t.Helper()
	root, err := project.CanonicalPath(t.TempDir())
	if err != nil {
		t.Fatalf("canonical path: %v", err)
	}
	return root
}

func mustProjectInfo(t *testing.T, root string) project.Info {
	t.Helper()
	return project.Info{
		ID:                 project.StableID(root),
		Root:               root,
		SupabaseDir:        filepath.Join(root, "supabase"),
		SourceConfigPath:   filepath.Join(root, "supabase", "config.toml"),
		RuntimeRoot:        project.RuntimeRoot(root),
		RuntimeSupabaseDir: project.RuntimeSupabaseDir(root),
		RuntimeConfigPath:  project.RuntimeConfigPath(root),
	}
}
