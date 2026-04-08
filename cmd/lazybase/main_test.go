package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"lazybase/internal/ports"
	"lazybase/internal/project"
	"lazybase/internal/registry"
	"lazybase/internal/supabase"
)

func TestLazybaseConfigDirUsesDotConfigUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := lazybaseConfigDir()
	if err != nil {
		fatalf(t, "lazybaseConfigDir: %v", err)
	}

	want := filepath.Join(home, ".config", "lazybase")
	if path != want {
		fatalf(t, "expected %q, got %q", want, path)
	}
}

func TestRunMainPropagatesSupabaseExitCode(t *testing.T) {
	prevRunner := runMainRunner
	runMainRunner = func([]string) error {
		return supabase.NewExitError(42, errors.New("wrapped command failed"))
	}
	t.Cleanup(func() {
		runMainRunner = prevRunner
	})

	exitCode := runMain([]string{"db", "push"})

	if exitCode != 42 {
		fatalf(t, "expected exit code 42, got %d", exitCode)
	}
}

func TestRunStartFromProjectRootUsesRuntimeWorkdir(t *testing.T) {
	projectRoot := t.TempDir()
	canonicalRoot := mustCanonicalPath(t, projectRoot)
	configPath := writeProjectConfig(t, projectRoot)
	seedRegistry(t, projectRoot, 2)
	writeProjectAsset(t, filepath.Join(projectRoot, "supabase", "migrations", "001.sql"), "create table test();\n")

	calledWorkdir := ""
	calledArgs := []string(nil)
	restore := stubRunStartDeps(projectRoot, func(workdir string, extraArgs []string) error {
		calledWorkdir = workdir
		calledArgs = append([]string(nil), extraArgs...)
		return nil
	})
	defer restore()

	if err := runStart([]string{"--debug"}); err != nil {
		fatalf(t, "runStart: %v", err)
	}

	runtimeRoot := project.RuntimeRoot(canonicalRoot)
	if calledWorkdir != runtimeRoot {
		fatalf(t, "expected workdir %q, got %q", runtimeRoot, calledWorkdir)
	}
	if len(calledArgs) != 1 || calledArgs[0] != "--debug" {
		fatalf(t, "unexpected extra args: %#v", calledArgs)
	}

	assertSourceConfigUnchanged(t, configPath)
	assertRuntimeConfigPorts(t, project.RuntimeConfigPath(canonicalRoot), ports.Compute(ports.Settings{Offset: ports.DefaultOffset}, 2, []ports.PortKey{ports.KeyAPIPort, ports.KeyStudioPort}))
	assertSymlinkTarget(t, filepath.Join(project.RuntimeSupabaseDir(canonicalRoot), "migrations"), filepath.Join(canonicalRoot, "supabase", "migrations"))
}

func TestRunStartFromSupabaseDirCanonicalizesProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	canonicalRoot := mustCanonicalPath(t, projectRoot)
	writeProjectConfig(t, projectRoot)
	seedRegistry(t, projectRoot, 1)

	supabaseDir := filepath.Join(projectRoot, "supabase")
	calledWorkdir := ""
	restore := stubRunStartDeps(supabaseDir, func(workdir string, extraArgs []string) error {
		calledWorkdir = workdir
		return nil
	})
	defer restore()

	if err := runStart(nil); err != nil {
		fatalf(t, "runStart: %v", err)
	}

	if calledWorkdir != project.RuntimeRoot(canonicalRoot) {
		fatalf(t, "expected workdir %q, got %q", project.RuntimeRoot(canonicalRoot), calledWorkdir)
	}

	configDir, err := lazybaseConfigDir()
	if err != nil {
		fatalf(t, "lazybaseConfigDir: %v", err)
	}

	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	reg, err := store.Load()
	if err != nil {
		fatalf(t, "load registry: %v", err)
	}
	if len(reg.Projects) != 1 {
		fatalf(t, "expected exactly 1 registry project, got %d", len(reg.Projects))
	}
	for _, entry := range reg.Projects {
		if entry.Path != canonicalRoot {
			fatalf(t, "expected registry path %q, got %q", canonicalRoot, entry.Path)
		}
	}
}

func stubRunStartDeps(cwd string, start func(string, []string) error) func() {
	prevGetwd := runStartGetwd
	prevLookPath := runStartLookPath
	prevSupabase := runStartSupabase

	runStartGetwd = func() (string, error) { return cwd, nil }
	runStartLookPath = func() (string, error) { return "supabase", nil }
	runStartSupabase = start

	return func() {
		runStartGetwd = prevGetwd
		runStartLookPath = prevLookPath
		runStartSupabase = prevSupabase
	}
}

func seedRegistry(t *testing.T, projectRoot string, slot int) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir, err := lazybaseConfigDir()
	if err != nil {
		fatalf(t, "lazybaseConfigDir: %v", err)
	}

	projectRoot, err = project.CanonicalPath(projectRoot)
	if err != nil {
		fatalf(t, "canonical path: %v", err)
	}
	id := project.StableID(projectRoot)
	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	if err := store.Save(&registry.Registry{Projects: map[string]registry.ProjectEntry{id: {ID: id, Path: projectRoot, Slot: slot}}}); err != nil {
		fatalf(t, "save registry: %v", err)
	}
}

func writeProjectConfig(t *testing.T, projectRoot string) string {
	t.Helper()
	supabaseDir := filepath.Join(projectRoot, "supabase")
	if err := os.MkdirAll(supabaseDir, 0o755); err != nil {
		fatalf(t, "mkdir supabase dir: %v", err)
	}

	configPath := filepath.Join(supabaseDir, "config.toml")
	raw := strings.Join([]string{
		"[api]",
		"port = 54321",
		"",
		"[studio]",
		"port = 54323",
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(raw), 0o644); err != nil {
		fatalf(t, "write config: %v", err)
	}
	return configPath
}

func writeProjectAsset(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatalf(t, "mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		fatalf(t, "write asset: %v", err)
	}
}

func assertSourceConfigUnchanged(t *testing.T, path string) {
	t.Helper()
	want := "[api]\nport = 54321\n\n[studio]\nport = 54323\n"
	if got := readFile(t, path); got != want {
		fatalf(t, "expected source config unchanged, got %q", got)
	}
}

func assertRuntimeConfigPorts(t *testing.T, path string, expected ports.PortMap) {
	t.Helper()
	text := readFile(t, path)
	if !strings.Contains(text, "port = "+itoa(expected[ports.KeyAPIPort])) {
		fatalf(t, "runtime config missing API port %d in %q", expected[ports.KeyAPIPort], text)
	}
	if !strings.Contains(text, "port = "+itoa(expected[ports.KeyStudioPort])) {
		fatalf(t, "runtime config missing Studio port %d in %q", expected[ports.KeyStudioPort], text)
	}
}

func assertSymlinkTarget(t *testing.T, path, wantTarget string) {
	t.Helper()
	gotTarget, err := os.Readlink(path)
	if err != nil {
		fatalf(t, "readlink %q: %v", path, err)
	}
	if gotTarget != wantTarget {
		fatalf(t, "expected symlink %q -> %q, got %q", path, wantTarget, gotTarget)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		fatalf(t, "read file %q: %v", path, err)
	}
	return string(raw)
}

func mustCanonicalPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := project.CanonicalPath(path)
	if err != nil {
		fatalf(t, "canonical path: %v", err)
	}
	return canonical
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func fatalf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Fatalf(format, args...)
}
