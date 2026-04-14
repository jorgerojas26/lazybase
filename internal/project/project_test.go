package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimePathsLiveUnderSupabaseLazybase(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := CanonicalPath(root)
	if err != nil {
		t.Fatalf("canonical path: %v", err)
	}

	wantRoot := filepath.Join(canonicalRoot, "supabase", ".lazybase", "runtime")
	if got := RuntimeRoot(canonicalRoot); got != wantRoot {
		t.Fatalf("expected runtime root %q, got %q", wantRoot, got)
	}

	wantSupabaseDir := filepath.Join(wantRoot, "supabase")
	if got := RuntimeSupabaseDir(canonicalRoot); got != wantSupabaseDir {
		t.Fatalf("expected runtime supabase dir %q, got %q", wantSupabaseDir, got)
	}

	wantConfigPath := filepath.Join(wantSupabaseDir, "config.toml")
	if got := RuntimeConfigPath(canonicalRoot); got != wantConfigPath {
		t.Fatalf("expected runtime config path %q, got %q", wantConfigPath, got)
	}
}

func TestResolveFromWorkingDirKeepsCanonicalRootWithRelocatedRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "supabase"), 0o755); err != nil {
		t.Fatalf("mkdir supabase dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "supabase", "config.toml"), []byte("[api]\nport = 54321\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	canonicalRoot, err := CanonicalPath(root)
	if err != nil {
		t.Fatalf("canonical path: %v", err)
	}

	for _, cwd := range []string{root, filepath.Join(root, "supabase")} {
		info, err := ResolveFromWorkingDir(cwd)
		if err != nil {
			t.Fatalf("resolve %q: %v", cwd, err)
		}
		if info.Root != canonicalRoot {
			t.Fatalf("expected canonical root %q, got %q", canonicalRoot, info.Root)
		}
		if info.RuntimeRoot != filepath.Join(canonicalRoot, "supabase", ".lazybase", "runtime") {
			t.Fatalf("unexpected runtime root %q", info.RuntimeRoot)
		}
	}
}
