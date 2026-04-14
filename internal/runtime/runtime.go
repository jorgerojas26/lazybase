package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

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

type runtimeConfig struct {
	DB struct {
		Seed struct {
			SQLPaths []string `toml:"sql_paths"`
		} `toml:"seed"`
	} `toml:"db"`
}

func Prepare(info project.Info, sourceConfig *configpkg.File, targetPorts ports.PortMap) error {
	if err := os.MkdirAll(info.RuntimeSupabaseDir, 0o755); err != nil {
		return fmt.Errorf("create runtime supabase dir: %w", err)
	}

	patched, _ := sourceConfig.PatchedBytes(targetPorts)
	if err := os.WriteFile(info.RuntimeConfigPath, patched, 0o644); err != nil {
		return fmt.Errorf("write runtime config: %w", err)
	}

	assetNames, warnings := linkedAssetNamesFromConfig(patched)
	for _, name := range assetNames {
		source := filepath.Join(info.SupabaseDir, name)
		if err := ensureSymlinkIfPresent(source, filepath.Join(info.RuntimeSupabaseDir, name)); err != nil {
			return err
		}
	}

	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "Lazybase: warning: %s\n", warning)
	}

	return nil
}

func linkedAssetNamesFromConfig(configRaw []byte) ([]string, []string) {
	names := make([]string, 0, len(linkedAssetNames)+2)
	seen := make(map[string]struct{}, len(linkedAssetNames)+2)

	for _, name := range linkedAssetNames {
		names = append(names, name)
		seen[name] = struct{}{}
	}

	extra, warnings := seedLinkedPaths(configRaw)
	sort.Strings(extra)
	for _, name := range extra {
		if _, ok := seen[name]; ok {
			continue
		}
		names = append(names, name)
		seen[name] = struct{}{}
	}

	return names, warnings
}

func seedLinkedPaths(configRaw []byte) ([]string, []string) {
	var cfg runtimeConfig
	if err := toml.Unmarshal(configRaw, &cfg); err != nil {
		return nil, nil
	}

	seen := make(map[string]struct{})
	paths := make([]string, 0, len(cfg.DB.Seed.SQLPaths))
	warnings := make([]string, 0)
	for _, sqlPath := range cfg.DB.Seed.SQLPaths {
		candidates, warning := linkCandidatesFromSQLPath(sqlPath)
		if warning != "" {
			warnings = append(warnings, fmt.Sprintf("ignored db.seed.sql_paths entry %q: %s", strings.TrimSpace(sqlPath), warning))
		}
		for _, candidate := range candidates {
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			paths = append(paths, candidate)
		}
	}

	return paths, warnings
}

func linkCandidatesFromSQLPath(sqlPath string) ([]string, string) {
	normalized := strings.TrimSpace(sqlPath)
	if normalized == "" {
		return nil, "empty path"
	}

	normalized = strings.TrimPrefix(normalized, "./")
	normalized = filepath.Clean(filepath.FromSlash(normalized))
	if normalized == "." || filepath.IsAbs(normalized) {
		if filepath.IsAbs(normalized) {
			return nil, "absolute paths are not supported"
		}
		return nil, "path resolves to current directory"
	}
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return nil, "path traversal outside supabase dir is not allowed"
	}

	seen := map[string]struct{}{}
	out := []string{}
	for _, expanded := range expandBracePatterns(normalized, 32) {
		literal := literalPrefixUntilGlob(expanded)
		if literal == "" || literal == "." {
			continue
		}
		if literal == ".." || strings.HasPrefix(literal, ".."+string(filepath.Separator)) {
			continue
		}
		if _, ok := seen[literal]; ok {
			continue
		}
		seen[literal] = struct{}{}
		out = append(out, literal)
	}

	if len(out) == 0 {
		return nil, "no linkable prefix found before glob tokens"
	}

	return out, ""
}

func expandBracePatterns(path string, max int) []string {
	if max <= 0 {
		return []string{path}
	}
	start, end, ok := firstBraceSegment(path)
	if !ok {
		return []string{path}
	}

	inside := path[start+1 : end]
	parts := splitTopLevelComma(inside)
	if len(parts) == 0 {
		return []string{path}
	}

	prefix := path[:start]
	suffix := path[end+1:]
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(out) >= max {
			break
		}
		next := prefix + part + suffix
		for _, expanded := range expandBracePatterns(next, max-len(out)) {
			out = append(out, expanded)
			if len(out) >= max {
				break
			}
		}
	}

	if len(out) == 0 {
		return []string{path}
	}
	return out
}

func firstBraceSegment(path string) (int, int, bool) {
	start := -1
	depth := 0
	for i, r := range path {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				return start, i, true
			}
		}
	}
	return 0, 0, false
}

func splitTopLevelComma(s string) []string {
	parts := []string{}
	depth := 0
	last := 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, s[last:])

	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		trimmed = append(trimmed, candidate)
	}
	return trimmed
}

func literalPrefixUntilGlob(path string) string {
	cleaned := filepath.Clean(path)
	parts := strings.Split(filepath.ToSlash(cleaned), "/")
	literalParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if strings.ContainsAny(part, "*?[]{}") {
			break
		}
		literalParts = append(literalParts, part)
	}

	if len(literalParts) == 0 {
		return ""
	}

	return filepath.FromSlash(strings.Join(literalParts, "/"))
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
