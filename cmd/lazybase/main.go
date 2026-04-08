package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	configpkg "lazybase/internal/config"
	"lazybase/internal/ports"
	"lazybase/internal/project"
	"lazybase/internal/registry"
	"lazybase/internal/runtime"
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

	projectInfo, err := resolveProjectRoot(cwd)
	if err != nil {
		return err
	}

	projectConfig, err := configpkg.ReadFile(projectInfo.SourceConfigPath)
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

	slot, reused, err := store.GetOrAllocate(reg, projectInfo, settings)
	if err != nil {
		return err
	}

	targetPorts := ports.Compute(settings, slot, activeKeys)
	if err := runtime.Prepare(projectInfo, projectConfig, targetPorts); err != nil {
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

	fmt.Fprintf(os.Stderr, "Lazybase: %s slot %d (runtime) studio=http://127.0.0.1:%d api=http://127.0.0.1:%d\n", verb, slot, studioPort, apiPort)

	return runStartSupabase(projectInfo.RuntimeRoot, extraArgs)
}

func resolveProjectRoot(cwd string) (project.Info, error) {
	return project.ResolveFromWorkingDir(cwd)
}

func lazybaseConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "lazybase"), nil
}
