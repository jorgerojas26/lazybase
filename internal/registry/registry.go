package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lazybase/internal/ports"
	"lazybase/internal/project"
)

const currentVersion = 2

type Store struct {
	path string
}

type Registry struct {
	Version  int                     `json:"version"`
	Projects map[string]ProjectEntry `json:"projects"`
	legacy   map[string]ProjectEntry
}

type ProjectEntry struct {
	ID              string `json:"id,omitempty"`
	Path            string `json:"path,omitempty"`
	Slot            int    `json:"slot"`
	LastUsedUnixSec int64  `json:"last_used_unix_sec,omitempty"`
}

type Project struct {
	ID              string
	Path            string
	Slot            int
	LastUsedUnixSec int64
	RuntimePath     string
}

type legacyRegistry struct {
	Version  int                     `json:"version"`
	Projects map[string]ProjectEntry `json:"projects"`
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (*Registry, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return newRegistry(), nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err == nil && looksLikeV2(reg) {
		normalizeRegistry(&reg)
		return &reg, nil
	}

	var legacy legacyRegistry
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}

	migrated := newRegistry()
	migrated.legacy = map[string]ProjectEntry{}
	for legacyPath, entry := range legacy.Projects {
		canonical := legacyPath
		if resolved, err := project.CanonicalPath(legacyPath); err == nil {
			canonical = resolved
		}
		id := project.StableID(canonical)
		migrated.Projects[id] = ProjectEntry{
			ID:              id,
			Path:            canonical,
			Slot:            entry.Slot,
			LastUsedUnixSec: entry.LastUsedUnixSec,
		}
		migrated.legacy[canonical] = ProjectEntry{ID: id, Path: canonical, Slot: entry.Slot, LastUsedUnixSec: entry.LastUsedUnixSec}
		if canonical != legacyPath {
			migrated.legacy[legacyPath] = ProjectEntry{ID: id, Path: canonical, Slot: entry.Slot, LastUsedUnixSec: entry.LastUsedUnixSec}
		}
	}
	return migrated, nil
}

func (s *Store) Save(reg *Registry) error {
	normalizeRegistry(reg)

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}

	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	raw = append(raw, '\n')

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return fmt.Errorf("write registry temp file: %w", err)
	}

	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("replace registry: %w", err)
	}

	reg.legacy = nil
	return nil
}

func (s *Store) GetOrAllocate(reg *Registry, projectInfo project.Info, settings ports.Settings) (int, bool, error) {
	normalizeRegistry(reg)
	if entry, ok := lookupProjectEntry(reg, projectInfo); ok {
		entry.LastUsedUnixSec = time.Now().Unix()
		reg.Projects[entry.ID] = entry
		return entry.Slot, true, nil
	}

	usedSlots := make(map[int]struct{}, len(reg.Projects))
	for _, entry := range reg.Projects {
		usedSlots[entry.Slot] = struct{}{}
	}

	for slot := 0; ; slot++ {
		if _, ok := usedSlots[slot]; ok {
			continue
		}

		portMap := ports.Compute(settings, slot, ports.ManagedKeys())
		if !ports.AllAvailable(portMap) {
			continue
		}

		reg.Projects[projectInfo.ID] = ProjectEntry{
			ID:              projectInfo.ID,
			Path:            projectInfo.Root,
			Slot:            slot,
			LastUsedUnixSec: time.Now().Unix(),
		}
		return slot, false, nil
	}
}

func (s *Store) Prune(reg *Registry, projectID string) bool {
	normalizeRegistry(reg)
	if _, ok := reg.Projects[projectID]; !ok {
		return false
	}
	delete(reg.Projects, projectID)
	return true
}

func (s *Store) List(reg *Registry) []Project {
	normalizeRegistry(reg)
	projects := make([]Project, 0, len(reg.Projects))
	for id, entry := range reg.Projects {
		path := entry.Path
		if path == "" {
			path = id
		}
		projects = append(projects, Project{
			ID:              id,
			Path:            path,
			Slot:            entry.Slot,
			LastUsedUnixSec: entry.LastUsedUnixSec,
			RuntimePath:     project.RuntimeRoot(path),
		})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Slot == projects[j].Slot {
			return strings.Compare(projects[i].Path, projects[j].Path) < 0
		}
		return projects[i].Slot < projects[j].Slot
	})
	return projects
}

func newRegistry() *Registry {
	return &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}}
}

func normalizeRegistry(reg *Registry) {
	if reg.Projects == nil {
		reg.Projects = map[string]ProjectEntry{}
	}
	normalized := make(map[string]ProjectEntry, len(reg.Projects))
	for id, entry := range reg.Projects {
		if entry.Path == "" {
			entry.Path = id
		}
		if entry.ID == "" {
			entry.ID = project.StableID(entry.Path)
		}
		normalized[entry.ID] = entry
	}
	reg.Version = currentVersion
	reg.Projects = normalized
	if reg.legacy == nil {
		reg.legacy = map[string]ProjectEntry{}
	}
	for _, entry := range reg.Projects {
		reg.legacy[entry.Path] = entry
	}
}

func looksLikeV2(reg Registry) bool {
	if reg.Version >= currentVersion {
		return true
	}
	for id, entry := range reg.Projects {
		if entry.ID != "" || entry.Path != "" {
			return true
		}
		if len(id) == 16 {
			return true
		}
	}
	return false
}

func lookupProjectEntry(reg *Registry, info project.Info) (ProjectEntry, bool) {
	if entry, ok := reg.Projects[info.ID]; ok {
		if entry.ID == "" {
			entry.ID = info.ID
		}
		if entry.Path == "" {
			entry.Path = info.Root
		}
		return entry, true
	}
	if entry, ok := reg.legacy[info.Root]; ok {
		entry.ID = info.ID
		entry.Path = info.Root
		delete(reg.legacy, info.Root)
		reg.Projects[info.ID] = entry
		return entry, true
	}
	return ProjectEntry{}, false
}
