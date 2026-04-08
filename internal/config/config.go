package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"lazybase/internal/ports"
)

type File struct {
	path       string
	raw        []byte
	activeKeys map[ports.PortKey]struct{}
}

type target struct {
	table string
	key   string
}

var managedTargets = map[ports.PortKey]target{
	ports.KeyAPIPort:           {table: "api", key: "port"},
	ports.KeyDBPort:            {table: "db", key: "port"},
	ports.KeyDBShadowPort:      {table: "db", key: "shadow_port"},
	ports.KeyDBPoolerPort:      {table: "db.pooler", key: "port"},
	ports.KeyStudioPort:        {table: "studio", key: "port"},
	ports.KeyInbucketPort:      {table: "inbucket", key: "port"},
	ports.KeyInbucketSMTPPort:  {table: "inbucket", key: "smtp_port"},
	ports.KeyInbucketPOP3Port:  {table: "inbucket", key: "pop3_port"},
	ports.KeyAnalyticsPort:     {table: "analytics", key: "port"},
	ports.KeyEdgeInspectorPort: {table: "edge_runtime", key: "inspector_port"},
}

var tablePattern = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(?:#.*)?$`)

func ReadFile(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var parsed any
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("validate config TOML: %w", err)
	}

	return &File{
		path:       path,
		raw:        raw,
		activeKeys: findActiveKeys(raw),
	}, nil
}

func (f *File) ActivePortKeys() []ports.PortKey {
	keys := make([]ports.PortKey, 0, len(f.activeKeys))
	for key := range f.activeKeys {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Compare(string(keys[i]), string(keys[j])) < 0
	})
	return keys
}

func (f *File) Patch(targetPorts ports.PortMap) (bool, error) {
	updated, changed := f.PatchedBytes(targetPorts)
	if !changed {
		return false, nil
	}

	if err := ensureBackup(f.path, f.raw); err != nil {
		return false, err
	}

	if err := os.WriteFile(f.path, updated, 0o644); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}

	f.raw = updated
	f.activeKeys = findActiveKeys(updated)
	return true, nil
}

func (f *File) PatchedBytes(targetPorts ports.PortMap) ([]byte, bool) {
	return patchRaw(bytes.Clone(f.raw), targetPorts)
}

func patchRaw(raw []byte, targetPorts ports.PortMap) ([]byte, bool) {
	lines := splitLines(string(raw))
	changed := false
	currentTable := ""

	for i, line := range lines {
		body, newline := splitNewline(line)
		trimmed := strings.TrimSpace(body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := tablePattern.FindStringSubmatch(body); len(matches) == 2 {
			currentTable = strings.TrimSpace(matches[1])
			continue
		}

		for key, spec := range managedTargets {
			if currentTable != spec.table {
				continue
			}

			port, ok := targetPorts[key]
			if !ok {
				continue
			}

			replaced, didReplace := replacePortValue(body, spec.key, port)
			if !didReplace {
				continue
			}

			if replaced != body {
				lines[i] = replaced + newline
				changed = true
			}
			break
		}
	}

	return []byte(strings.Join(lines, "")), changed
}

func findActiveKeys(raw []byte) map[ports.PortKey]struct{} {
	lines := splitLines(string(raw))
	active := make(map[ports.PortKey]struct{})
	currentTable := ""

	for _, line := range lines {
		body, _ := splitNewline(line)
		trimmed := strings.TrimSpace(body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := tablePattern.FindStringSubmatch(body); len(matches) == 2 {
			currentTable = strings.TrimSpace(matches[1])
			continue
		}

		for key, spec := range managedTargets {
			if currentTable != spec.table {
				continue
			}
			if hasActiveAssignment(body, spec.key) {
				active[key] = struct{}{}
			}
		}
	}

	return active
}

func hasActiveAssignment(line, key string) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`^(\s*%s\s*=\s*)(\d+)(\s*(?:#.*)?)$`, regexp.QuoteMeta(key)))
	return pattern.MatchString(line)
}

func replacePortValue(line, key string, port int) (string, bool) {
	pattern := regexp.MustCompile(fmt.Sprintf(`^(\s*%s\s*=\s*)(\d+)(\s*(?:#.*)?)$`, regexp.QuoteMeta(key)))
	matches := pattern.FindStringSubmatch(line)
	if len(matches) != 4 {
		return line, false
	}
	return matches[1] + fmt.Sprintf("%d", port) + matches[3], true
}

func ensureBackup(path string, raw []byte) error {
	backupPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".bak")
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config backup: %w", err)
	}

	if err := os.WriteFile(backupPath, raw, 0o644); err != nil {
		return fmt.Errorf("create config backup: %w", err)
	}
	return nil
}

func splitLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.SplitAfter(raw, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func splitNewline(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return strings.TrimSuffix(line, "\r\n"), "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return strings.TrimSuffix(line, "\n"), "\n"
	}
	return line, ""
}

func PatchRawForTests(raw []byte, targetPorts ports.PortMap) ([]byte, bool) {
	patched, changed := patchRaw(bytes.Clone(raw), targetPorts)
	return patched, changed
}
