package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	configpkg "lazybase/internal/config"
	"lazybase/internal/ports"
	"lazybase/internal/project"
)

var linkedAssetNames = []string{
	"migrations",
	"functions",
	"tests",
	"seed.sql",
	"templates",
	"fixtures",
}

func Prepare(info project.Info, sourceConfig *configpkg.File, targetPorts ports.PortMap) error {
	if err := os.MkdirAll(info.RuntimeSupabaseDir, 0o755); err != nil {
		return fmt.Errorf("create runtime supabase dir: %w", err)
	}

	patched, _ := sourceConfig.PatchedBytes(targetPorts)
	if err := os.WriteFile(info.RuntimeConfigPath, patched, 0o644); err != nil {
		return fmt.Errorf("write runtime config: %w", err)
	}

	for _, name := range linkedAssetNames {
		source := filepath.Join(info.SupabaseDir, name)
		if err := ensureSymlinkIfPresent(source, filepath.Join(info.RuntimeSupabaseDir, name)); err != nil {
			return err
		}
	}

	return nil
}

func ensureSymlinkIfPresent(source, target string) error {
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove stale runtime asset %s: %w", target, err)
			}
			return nil
		}
		return fmt.Errorf("stat runtime asset %s: %w", source, err)
	}

	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace runtime asset %s: %w", target, err)
	}

	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("symlink runtime asset %s: %w", filepath.Base(target), err)
	}

	return nil
}
