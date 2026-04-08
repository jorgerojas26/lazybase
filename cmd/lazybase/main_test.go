package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "lazybase/internal/config"
	"lazybase/internal/ports"
	"lazybase/internal/registry"
	"lazybase/internal/supabase"
)

func TestLazybaseConfigDirUsesDotConfigUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := lazybaseConfigDir()
	if err != nil {
		t.Fatalf("lazybaseConfigDir: %v", err)
	}

	want := filepath.Join(home, ".config", "lazybase")
	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
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
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}

func TestRunStartFromProjectRootUsesProjectWorkdir(t *testing.T) {
	projectRoot := t.TempDir()
	supabaseDir := filepath.Join(projectRoot, "supabase")
	if err := os.MkdirAll(supabaseDir, 0o755); err != nil {
		t.Fatalf("mkdir supabase dir: %v", err)
	}

	configPath := filepath.Join(supabaseDir, "config.toml")
	writeTestConfig(t, configPath)
	seedRegistry(t, projectRoot, 2)

	calledWorkdir := ""
	calledArgs := []string(nil)
	restore := stubRunStartDeps(projectRoot, func(workdir string, extraArgs []string) error {
		calledWorkdir = workdir
		calledArgs = append([]string(nil), extraArgs...)
		return nil
	})
	defer restore()

	if err := runStart([]string{"--debug"}); err != nil {
		t.Fatalf("runStart: %v", err)
	}

	if calledWorkdir != projectRoot {
		t.Fatalf("expected workdir %q, got %q", projectRoot, calledWorkdir)
	}
	if len(calledArgs) != 1 || calledArgs[0] != "--debug" {
		t.Fatalf("unexpected extra args: %#v", calledArgs)
	}

	assertConfigPorts(t, configPath, ports.Compute(ports.Settings{Offset: ports.DefaultOffset}, 2, []ports.PortKey{ports.KeyAPIPort, ports.KeyStudioPort}))
}

func TestRunStartFromSupabaseDirCanonicalizesProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	supabaseDir := filepath.Join(projectRoot, "supabase")
	if err := os.MkdirAll(supabaseDir, 0o755); err != nil {
		t.Fatalf("mkdir supabase dir: %v", err)
	}

	configPath := filepath.Join(supabaseDir, "config.toml")
	writeTestConfig(t, configPath)
	seedRegistry(t, projectRoot, 1)

	calledWorkdir := ""
	restore := stubRunStartDeps(supabaseDir, func(workdir string, extraArgs []string) error {
		calledWorkdir = workdir
		return nil
	})
	defer restore()

	if err := runStart(nil); err != nil {
		t.Fatalf("runStart: %v", err)
	}

	if calledWorkdir != projectRoot {
		t.Fatalf("expected workdir %q, got %q", projectRoot, calledWorkdir)
	}

	configDir, err := lazybaseConfigDir()
	if err != nil {
		t.Fatalf("lazybaseConfigDir: %v", err)
	}

	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	reg, err := store.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if _, ok := reg.Projects[projectRoot]; !ok {
		t.Fatalf("expected registry entry for canonical root %q", projectRoot)
	}
	if _, ok := reg.Projects[supabaseDir]; ok {
		t.Fatalf("did not expect registry entry for %q", supabaseDir)
	}

	assertConfigPorts(t, configPath, ports.Compute(ports.Settings{Offset: ports.DefaultOffset}, 1, []ports.PortKey{ports.KeyAPIPort, ports.KeyStudioPort}))
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
		t.Fatalf("lazybaseConfigDir: %v", err)
	}

	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	if err := store.Save(&registry.Registry{Projects: map[string]registry.ProjectEntry{projectRoot: {Slot: slot}}}); err != nil {
		t.Fatalf("save registry: %v", err)
	}
}

func writeTestConfig(t *testing.T, path string) {
	t.Helper()

	raw := strings.Join([]string{
		"[api]",
		"port = 54321",
		"",
		"[studio]",
		"port = 54323",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func assertConfigPorts(t *testing.T, path string, expected ports.PortMap) {
	t.Helper()

	file, err := configpkg.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	patched, changed := configpkg.PatchRawForTests([]byte(readFile(t, path)), expected)
	if changed {
		t.Fatalf("expected config %q to already be patched, next patch would produce %q", path, string(patched))
	}

	activeKeys := file.ActivePortKeys()
	if len(activeKeys) != 2 {
		t.Fatalf("expected 2 active keys, got %v", activeKeys)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %q: %v", path, err)
	}
	return string(raw)
}
