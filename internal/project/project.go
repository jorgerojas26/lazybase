package project

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Info struct {
	ID                 string
	Root               string
	SupabaseDir        string
	SourceConfigPath   string
	RuntimeRoot        string
	RuntimeSupabaseDir string
	RuntimeConfigPath  string
}

func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	abs = filepath.Clean(abs)
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return abs, nil
	}
	return "", err
}

func StableID(path string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(path)))
	return hex.EncodeToString(sum[:8])
}

func ResolveFromWorkingDir(cwd string) (Info, error) {
	current, err := CanonicalPath(cwd)
	if err != nil {
		return Info{}, err
	}

	if info, ok, err := infoFromRootCandidate(current); err != nil {
		return Info{}, err
	} else if ok {
		return info, nil
	}

	if filepath.Base(current) == "supabase" {
		parent := filepath.Dir(current)
		if info, ok, err := infoFromSupabaseDir(parent, current); err != nil {
			return Info{}, err
		} else if ok {
			return info, nil
		}
	}

	return Info{}, fmt.Errorf("missing supabase/config.toml in %s", current)
}

func RuntimeRoot(root string) string {
	return filepath.Join(root, "supabase", ".lazybase", "runtime")
}

func RuntimeSupabaseDir(root string) string {
	return filepath.Join(RuntimeRoot(root), "supabase")
}

func RuntimeConfigPath(root string) string {
	return filepath.Join(RuntimeSupabaseDir(root), "config.toml")
}

func infoFromRootCandidate(root string) (Info, bool, error) {
	supabaseDir := filepath.Join(root, "supabase")
	configPath := filepath.Join(supabaseDir, "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		return buildInfo(root, supabaseDir, configPath), true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Info{}, false, err
	}
	return Info{}, false, nil
}

func infoFromSupabaseDir(root, supabaseDir string) (Info, bool, error) {
	configPath := filepath.Join(supabaseDir, "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		return buildInfo(root, supabaseDir, configPath), true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Info{}, false, err
	}
	return Info{}, false, nil
}

func buildInfo(root, supabaseDir, configPath string) Info {
	return Info{
		ID:                 StableID(root),
		Root:               root,
		SupabaseDir:        supabaseDir,
		SourceConfigPath:   configPath,
		RuntimeRoot:        RuntimeRoot(root),
		RuntimeSupabaseDir: RuntimeSupabaseDir(root),
		RuntimeConfigPath:  RuntimeConfigPath(root),
	}
}
