package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"lazybase/internal/ports"
)

const currentVersion = 1

type Store struct {
	path string
}

type Registry struct {
	Version  int                     `json:"version"`
	Projects map[string]ProjectEntry `json:"projects"`
}

type ProjectEntry struct {
	Slot int `json:"slot"`
}

type Project struct {
	Path string
	Slot int
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (*Registry, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Version: currentVersion, Projects: map[string]ProjectEntry{}}, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}

	if reg.Version == 0 {
		reg.Version = currentVersion
	}
	if reg.Projects == nil {
		reg.Projects = map[string]ProjectEntry{}
	}

	return &reg, nil
}

func (s *Store) Save(reg *Registry) error {
	if reg.Projects == nil {
		reg.Projects = map[string]ProjectEntry{}
	}
	reg.Version = currentVersion

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

	return nil
}

func (s *Store) GetOrAllocate(reg *Registry, projectPath string, settings ports.Settings) (int, bool, error) {
	if entry, ok := reg.Projects[projectPath]; ok {
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

		reg.Projects[projectPath] = ProjectEntry{Slot: slot}
		return slot, false, nil
	}
}

func (s *Store) Prune(reg *Registry, projectPath string) bool {
	if _, ok := reg.Projects[projectPath]; !ok {
		return false
	}
	delete(reg.Projects, projectPath)
	return true
}

func (s *Store) List(reg *Registry) []Project {
	projects := make([]Project, 0, len(reg.Projects))
	for path, entry := range reg.Projects {
		projects = append(projects, Project{Path: path, Slot: entry.Slot})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Slot == projects[j].Slot {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Slot < projects[j].Slot
	})
	return projects
}
