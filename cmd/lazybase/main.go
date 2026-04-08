package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	configpkg "lazybase/internal/config"
	"lazybase/internal/ports"
	"lazybase/internal/registry"
	"lazybase/internal/supabase"
	"lazybase/internal/tui"
)

var (
	runMainRunner    = run
	runStartGetwd    = os.Getwd
	runStartLookPath = supabase.LookPath
	runStartSupabase = supabase.StartWithWorkdir
)

func main() {
	os.Exit(runMain(os.Args[1:]))
}

func runMain(args []string) int {
	if err := runMainRunner(args); err != nil {
		var exitErr supabase.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "lazybase:", err)
		return 1
	}

	return 0
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}

	if args[0] == "start" {
		return runStart(args[1:])
	}

	return supabase.Run(args)
}

func runTUI() error {
	configDir, err := lazybaseConfigDir()
	if err != nil {
		return err
	}

	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	settings, err := ports.LoadSettings(filepath.Join(configDir, "lazybase.yaml"))
	if err != nil {
		return err
	}

	return tui.Run(store, settings)
}

func runStart(extraArgs []string) error {
	if _, err := runStartLookPath(); err != nil {
		return err
	}

	cwd, err := runStartGetwd()
	if err != nil {
		return err
	}

	projectPath, configPath, err := resolveProjectRoot(cwd)
	if err != nil {
		return err
	}

	projectConfig, err := configpkg.ReadFile(configPath)
	if err != nil {
		return err
	}

	activeKeys := projectConfig.ActivePortKeys()
	if len(activeKeys) == 0 {
		return errors.New("no supported Lazybase-managed ports found in supabase/config.toml")
	}

	configDir, err := lazybaseConfigDir()
	if err != nil {
		return err
	}

	settings, err := ports.LoadSettings(filepath.Join(configDir, "lazybase.yaml"))
	if err != nil {
		return err
	}

	store := registry.NewStore(filepath.Join(configDir, "registry.json"))
	reg, err := store.Load()
	if err != nil {
		return err
	}

	slot, reused, err := store.GetOrAllocate(reg, projectPath, settings)
	if err != nil {
		return err
	}

	targetPorts := ports.Compute(settings, slot, activeKeys)
	changed, err := projectConfig.Patch(targetPorts)
	if err != nil {
		return err
	}

	if err := store.Save(reg); err != nil {
		return err
	}

	apiPort := targetPorts[ports.KeyAPIPort]
	studioPort := targetPorts[ports.KeyStudioPort]
	verb := "allocated"
	if reused {
		verb = "reused"
	}
	status := "unchanged"
	if changed {
		status = "patched"
	}

	fmt.Fprintf(os.Stderr, "Lazybase: %s slot %d (%s) studio=http://127.0.0.1:%d api=http://127.0.0.1:%d\n", verb, slot, status, studioPort, apiPort)

	return runStartSupabase(projectPath, extraArgs)
}

func resolveProjectRoot(cwd string) (string, string, error) {
	projectPath, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", err
	}

	configPath := filepath.Join(projectPath, "supabase", "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		return projectPath, configPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	if filepath.Base(projectPath) == "supabase" {
		configPath = filepath.Join(projectPath, "config.toml")
		if _, err := os.Stat(configPath); err == nil {
			return filepath.Dir(projectPath), configPath, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
	}

	return "", "", fmt.Errorf("missing supabase/config.toml in %s", projectPath)
}

func lazybaseConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "lazybase"), nil
}
